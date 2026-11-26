package httpapp

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/StephenCotterrell/azulwithfriends/internal/database/dbtest"
	"github.com/StephenCotterrell/azulwithfriends/internal/game"
)

func TestGuestStartsPrivateSoloGame(t *testing.T) {
	pool := openTestDB(t, dbtest.IsolatedDSN(t))
	handler := New(pool, t.TempDir(), true)
	const id = "c31b16ac-f8fa-40c5-a82f-5a9ceebdadf1"
	const path = "/api/solo-games"
	const body = `{"id":"` + id + `"}`
	if got := sendRequest(handler, "POST", path, "{}", nil); got.Code != http.StatusBadRequest {
		t.Fatalf("missing request ID: %d %s", got.Code, got.Body)
	}

	// Concurrent taps from a fresh browser must create only one room and guest.
	results := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { results <- sendRequest(handler, "POST", path, body, nil) }()
	}
	created, duplicate := <-results, <-results
	if duplicate.Code == http.StatusCreated {
		created, duplicate = duplicate, created
	}
	if created.Code != http.StatusCreated || duplicate.Code != http.StatusConflict || len(duplicate.Result().Cookies()) != 0 {
		t.Fatalf("concurrent starts returned %d and %d", created.Code, duplicate.Code)
	}
	room := readGameRoom(t, created)
	cookies := created.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || len(room.Seats) != 2 ||
		room.ID != id || room.Listed || room.YourSeat != 1 || room.Seats[0].IsBot ||
		!room.Seats[1].IsBot || room.Seats[1].BotLevel != game.BotNoob {
		t.Fatalf("unexpected solo room: %+v", room)
	}
	if room.Game == nil || room.Game.Round != 1 || room.Game.Turn != 1 || room.Game.Finished || len(room.Game.LegalMoves) == 0 {
		t.Fatalf("solo game is not ready for the host: %+v", room.Game)
	}
	if location := created.Header().Get("Location"); location != "/room/"+room.Slug {
		t.Fatalf("incorrect board location: %q", location)
	}

	host := cookies[0]
	lobby := sendRequest(handler, "GET", "/api/lobby", "", host)
	if want := `{"rooms":[],"resumeGame":"` + room.Slug + `"}` + "\n"; lobby.Code != http.StatusOK || lobby.Body.String() != want {
		t.Fatalf("private game missing from resume or present in lobby: %d %s", lobby.Code, lobby.Body)
	}
	url := "/api/rooms/" + room.Slug
	if got := readGameRoom(t, sendRequest(handler, "GET", url, "", host)); !reflect.DeepEqual(got, room) {
		t.Fatal("loading the board did not restore the solo game")
	}
	if got := readGameRoom(t, sendRequest(handler, "GET", url, "", nil)); got.YourSeat != 0 || got.Game != nil {
		t.Fatal("anonymous visitor received the private board")
	}
	move := moveBody(t, room.Game.Revision, shortRowMove(room.Game.LegalMoves))
	played := readGameRoom(t, sendRequest(handler, "POST", url+"/moves", move, host))
	if played.Game.Revision <= room.Game.Revision || played.Game.LastMove == nil || played.Game.LastMove.Seat != 2 {
		t.Fatal("the bot did not respond to a draft")
	}
	retry := sendRequest(handler, "POST", path, body, host)
	if got := readGameRoom(t, retry); !reflect.DeepEqual(got, played) || len(retry.Result().Cookies()) != 0 {
		t.Fatal("retry reset the board or changed the guest")
	}
	var rooms, players, sessions int
	err := pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM rooms), (SELECT count(*) FROM players), (SELECT count(*) FROM sessions)").Scan(&rooms, &players, &sessions)
	if err != nil || rooms != 1 || players != 1 || sessions != 1 {
		t.Fatalf("duplicate start left %d rooms, %d players and %d sessions: %v", rooms, players, sessions, err)
	}
}

func TestSoloStartRollsBackOnFailure(t *testing.T) {
	pool := openTestDB(t, dbtest.IsolatedDSN(t))
	handler := New(pool, t.TempDir(), true)
	const endpoint = "/api/solo-games"
	const body = `{"id":"e6cbdaf2-bf1a-4855-a8c2-f85d2474e10a"}`

	if _, err := pool.Exec(t.Context(), "ALTER TABLE room_seats ADD CONSTRAINT test_block_bot CHECK (NOT is_bot)"); err != nil {
		t.Fatal(err)
	}
	failed := sendRequest(handler, "POST", endpoint, body, nil)
	if failed.Code != http.StatusServiceUnavailable || len(failed.Result().Cookies()) != 0 {
		t.Fatalf("unexpected failed start: %d %s", failed.Code, failed.Body)
	}
	var rooms, players, sessions int
	err := pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM rooms), (SELECT count(*) FROM players), (SELECT count(*) FROM sessions)").Scan(&rooms, &players, &sessions)
	if err != nil || rooms != 0 || players != 0 || sessions != 0 {
		t.Fatalf("failed start left %d rooms, %d players and %d sessions: %v", rooms, players, sessions, err)
	}
	if _, err := pool.Exec(t.Context(), "ALTER TABLE room_seats DROP CONSTRAINT test_block_bot"); err != nil {
		t.Fatal(err)
	}
	created := sendRequest(handler, "POST", endpoint, body, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("retry failed: %d %s", created.Code, created.Body)
	}
}
