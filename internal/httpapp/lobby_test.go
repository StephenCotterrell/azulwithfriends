package httpapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/database/dbtest"
)

func TestHostListingAndGuestLobbyJoin(t *testing.T) {
	pool := openTestDB(t, dbtest.IsolatedDSN(t))
	handler := New(pool, t.TempDir(), true)
	post := func(path, body string, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		t.Helper()
		rec := sendRequest(handler, "POST", path, body, cookie)
		if rec.Code != status {
			t.Fatalf("POST %s returned %d, want %d: %s", path, rec.Code, status, rec.Body)
		}
		return rec
	}
	// Keep the lobby projection small. Unknown fields would expose more room
	// or player information to unseated visitors than the listing requires.
	type entry struct {
		Slug     string `json:"slug"`
		HostName string `json:"hostName"`
		IsHost   bool   `json:"isHost"`
	}
	checkLobby := func(cookie *http.Cookie, want ...entry) {
		t.Helper()
		rec := sendRequest(handler, "GET", "/api/lobby", "", cookie)
		var lobby struct {
			Rooms []entry `json:"rooms"`
		}
		decoder := json.NewDecoder(rec.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&lobby); err != nil || rec.Code != 200 ||
			lobby.Rooms == nil || !slices.Equal(lobby.Rooms, want) ||
			len(rec.Result().Cookies()) != 0 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected lobby: status %d, rooms %+v, error %v; want %+v", rec.Code, lobby.Rooms, err, want)
		}
	}
	checkLobby(nil)
	created := post("/api/rooms", `{"listed":true}`, nil, 201)
	var room database.Room
	if err := json.Unmarshal(created.Body.Bytes(), &room); err != nil || !room.Listed || !roomSlugPattern.MatchString(room.Slug) {
		t.Fatalf("unexpected public room: %+v, %v", room, err)
	}
	host := created.Result().Cookies()[0]
	bySlug := "/api/rooms/" + room.Slug
	byID := "/api/rooms/" + room.ID
	checkLobby(nil, entry{room.Slug, "Player 1", false})
	post("/api/session", `{"displayName":"Robin"}`, host, 200)
	checkLobby(host, entry{room.Slug, "Robin", true})
	post(bySlug+"/listing", `{"listed":false}`, nil, 403)
	post(byID+"/listing", `{"listed":false}`, host, 200)
	checkLobby(nil)

	// A stale Join button must not reserve an unlisted seat or issue a guest
	// cookie. A host can make the same link discoverable again.
	stale := post(bySlug+"/join", `{"fromLobby":true}`, nil, 409)
	if len(stale.Result().Cookies()) != 0 {
		t.Fatal("stale lobby join created a guest cookie")
	}
	var players, sessions, seats int
	if err := pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM players), (SELECT count(*) FROM sessions), (SELECT count(*) FROM room_seats)").Scan(&players, &sessions, &seats); err != nil || players != 1 || sessions != 1 || seats != 1 {
		t.Fatalf("stale lobby join changed row counts: players %d, sessions %d, seats %d, err %v", players, sessions, seats, err)
	}
	post(bySlug+"/listing", `{"listed":true}`, host, 200)
	checkLobby(nil, entry{room.Slug, "Robin", false})
	joined := post(bySlug+"/join", `{"fromLobby":true}`, nil, 200)
	friend := joined.Result().Cookies()[0]
	var view database.Room
	if err := json.Unmarshal(joined.Body.Bytes(), &view); err != nil || view.ID != room.ID || view.YourSeat != 2 {
		t.Fatalf("unexpected guest join: %+v, %v", view, err)
	}
	checkLobby(nil)
	post(bySlug+"/join", `{"fromLobby":true}`, nil, 409)
	post(bySlug+"/listing", `{"listed":false}`, friend, 403)
	retry := post(bySlug+"/join", `{"fromLobby":true}`, friend, 200)
	if len(retry.Result().Cookies()) != 0 {
		t.Fatal("a repeat lobby join replaced the guest's cookie")
	}
	post(bySlug+"/start", "{}", host, 200)
	checkLobby(nil)
}

func TestLobbyResumesOnlyTheGuestsUnfinishedGame(t *testing.T) {
	pool := openTestDB(t, dbtest.IsolatedDSN(t))
	handler := New(pool, t.TempDir(), true)
	check := func(cookie *http.Cookie, preferred, want string) {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/lobby", nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		req.Header.Set("X-Recent-Game", preferred)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		var lobby struct {
			ResumeGame string `json:"resumeGame"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &lobby); err != nil || rec.Code != http.StatusOK || lobby.ResumeGame != want {
			t.Fatalf("lobby status %d, resume %q; want %q (decode error %v)", rec.Code, lobby.ResumeGame, want, err)
		}
	}

	created := sendRequest(handler, "POST", "/api/rooms", "{}", nil)
	first := readGameRoom(t, created)
	host := created.Result().Cookies()[0]
	check(host, first.Slug, "")
	joined := sendRequest(handler, "POST", "/api/rooms/"+first.Slug+"/join", "{}", nil)
	guest := joined.Result().Cookies()[0]
	readGameRoom(t, sendRequest(handler, "POST", "/api/rooms/"+first.Slug+"/start", "{}", host))
	second := readGameRoom(t, sendRequest(handler, "POST", "/api/rooms", "{}", host))
	readGameRoom(t, sendRequest(handler, "POST", "/api/rooms/"+second.Slug+"/bot", `{"enabled":true}`, host))
	readGameRoom(t, sendRequest(handler, "POST", "/api/rooms/"+second.Slug+"/start", "{}", host))

	check(host, "", second.Slug)
	check(host, first.Slug, first.Slug)
	check(guest, second.Slug, first.Slug)
	check(nil, first.Slug, "")
	if _, err := pool.Exec(t.Context(), `UPDATE rooms SET game_state = jsonb_set(game_state, '{finished}', 'true') WHERE id = $1`, second.ID); err != nil {
		t.Fatal(err)
	}
	check(host, second.Slug, first.Slug)
	if _, err := pool.Exec(t.Context(), "UPDATE sessions SET expires_at = now()"); err != nil {
		t.Fatal(err)
	}
	check(host, first.Slug, "")
}
