package httpapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/StephenCotterrell/azulwithfriends/internal/database/dbtest"
)

func TestRoomLinksAndInvalidRequests(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<title>Room</title>"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := New(nil, dist, true)
	for _, path := range []string{"/r/bad-link", "/r/bad-link/"} {
		invite := sendRequest(handler, "GET", path, "", nil)
		if invite.Code != 200 || !strings.Contains(invite.Body.String(), "<title>Room</title>") ||
			invite.Header().Get("Referrer-Policy") != "no-referrer" || len(invite.Result().Cookies()) != 0 {
			t.Errorf("GET %s should serve the private link's shell without creating a guest", path)
		}
	}
	for _, path := range []string{"/assets/missing.js", "/somewhere-else", "/api/rooms/not-a-uuid"} {
		rec := sendRequest(handler, "GET", path, "", nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: got %d", path, rec.Code)
		}
	}
	for _, tc := range []struct {
		path, body, origin, contentType string
		want                            int
	}{
		{"/api/rooms", "{}", "https://other.example", "application/json", 403},
		{"/api/rooms/a4682816-f1a3-4d82-9f25-7b7fea55dd28/join", "{}", "https://other.example", "application/json", 403},
		{"/api/rooms", "", "", "text/plain", 415},
		{"/api/rooms", "null", "", "application/json", 400},
		{"/api/rooms", `{"yourSeat":1}`, "", "application/json", 400},
		{"/api/rooms/not-a-uuid/join", "{}", "", "application/json", 404},
		{"/api/rooms/a4682816-f1a3-4d82-9f25-7b7fea55dd28/start", "{}", "https://other.example", "application/json", 403},
		{"/api/rooms/a4682816-f1a3-4d82-9f25-7b7fea55dd28/start", "null", "", "application/json", 400},
		{"/api/rooms/a4682816-f1a3-4d82-9f25-7b7fea55dd28/moves", `{"revision":1,"color":"red","row":0}`, "", "application/json", 400},
		{"/api/rooms/a4682816-f1a3-4d82-9f25-7b7fea55dd28/moves", `{"revision":1,"color":"red","source":0}`, "", "application/json", 400},
		{"/api/rooms/a4682816-f1a3-4d82-9f25-7b7fea55dd28/moves", `{"revision":1,"color":"red","source":0,"row":0,"seat":1}`, "", "application/json", 400},
	} {
		req := httptest.NewRequest("POST", "https://game.example"+tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", tc.contentType)
		req.Header.Set("Origin", tc.origin)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want || len(rec.Result().Cookies()) != 0 {
			t.Errorf("POST %s (%s): got %d, want %d without a cookie", tc.path, tc.body, rec.Code, tc.want)
		}
	}
}

func TestAnonymousRoomInvitation(t *testing.T) {
	pool := openTestDB(t, dbtest.IsolatedDSN(t))
	handler := New(pool, t.TempDir(), true)
	send := func(method, endpoint, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		return sendRequest(handler, method, endpoint, body, cookie)
	}
	// Limit accepted fields to the public contract; private player IDs and
	// session tokens must never be serialized into the shared room view.
	type seat struct {
		Number      int    `json:"number"`
		DisplayName string `json:"displayName"`
	}
	type view struct {
		ID       string          `json:"id"`
		Seats    []seat          `json:"seats"`
		YourSeat int             `json:"yourSeat"`
		Game     json.RawMessage `json:"game"`
	}
	readView := func(rec *httptest.ResponseRecorder, status, yourSeat int, wantSeats ...seat) view {
		t.Helper()
		var v view
		decoder := json.NewDecoder(rec.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&v); err != nil || rec.Code != status || v.YourSeat != yourSeat ||
			!roomIDPattern.MatchString(v.ID) || !slices.Equal(v.Seats, wantSeats) || string(v.Game) != "null" ||
			rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("got %d, %+v, error %v; want %d, seat %d, roster %+v", rec.Code, v, err, status, yourSeat, wantSeats)
		}
		return v
	}
	guestCookie := func(rec *httptest.ResponseRecorder) *http.Cookie {
		t.Helper()
		cookies := rec.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != "__Host-azul_guest" || !cookies[0].HttpOnly ||
			!cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/" {
			t.Fatal("new seat must issue one protected cookie")
		}
		return cookies[0]
	}

	created := send("POST", "/api/rooms", "{}", nil)
	first := seat{1, "Player 1"}
	room := readView(created, 201, 1, first)
	host := guestCookie(created)
	api := "/api/rooms/" + room.ID
	if created.Header().Get("Location") != "/r/"+room.ID {
		t.Fatal("missing shareable room URL")
	}
	preview := send("GET", api, "", nil)
	readView(preview, 200, 0, first)
	if len(preview.Result().Cookies()) != 0 {
		t.Fatal("reading the invitation created a session")
	}
	var players, sessions int
	if err := pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM players), (SELECT count(*) FROM sessions)").Scan(&players, &sessions); err != nil || players != 1 || sessions != 1 {
		t.Fatalf("unexpected records after invitation preview: %d players, %d sessions, err %v", players, sessions, err)
	}

	second := seat{2, "Player 2"}
	joined := send("POST", api+"/join", "{}", nil)
	readView(joined, 200, 2, first, second)
	friend := guestCookie(joined)
	if friend.Value == host.Value {
		t.Fatal("host and friend's credentials must be independent")
	}
	renamed := send("POST", "/api/session", `{"displayName":"Lena"}`, friend)
	if renamed.Code != 200 || len(renamed.Result().Cookies()) != 0 {
		t.Fatal("optional rename must keep existing session")
	}
	second.DisplayName = "Lena"
	readView(send("GET", api, "", host), 200, 1, first, second)
	readView(send("GET", api, "", friend), 200, 2, first, second)
	rejoin := send("POST", api+"/join", "{}", friend)
	readView(rejoin, 200, 2, first, second)
	if len(rejoin.Result().Cookies()) != 0 {
		t.Fatal("returning member must keep their original session")
	}
	rejected := send("POST", api+"/join", "{}", nil)
	if rejected.Code != 409 || len(rejected.Result().Cookies()) != 0 {
		t.Fatal("full room must decline a third seat without creating a guest")
	}
	readView(send("GET", api, "", nil), 200, 0, first, second)
	newRoom := send("POST", "/api/rooms", "{}", friend)
	readView(newRoom, 201, 1, seat{1, "Lena"})
	if len(newRoom.Result().Cookies()) != 0 {
		t.Fatal("guest with an existing name shouldn't need a new identity to create a room")
	}
}
