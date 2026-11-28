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
	const body = `{"id":"` + id + `","level":"intermediate"}`
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
		!room.Seats[1].IsBot || room.Seats[1].BotLevel != game.BotIntermediate {
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
	// A lost response can be retried with this UUID. Even an older client or a
	// conflicting selection must get the original personality and latest board.
	for _, retryBody := range []string{body, `{"id":"` + id + `"}`, `{"id":"` + id + `","level":"master"}`} {
		retry := sendRequest(handler, "POST", path, retryBody, host)
		if got := readGameRoom(t, retry); !reflect.DeepEqual(got, played) || len(retry.Result().Cookies()) != 0 {
			t.Fatalf("retry %s changed the game or guest", retryBody)
		}
	}
	var rooms, players, sessions int
	err := pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM rooms), (SELECT count(*) FROM players), (SELECT count(*) FROM sessions)").Scan(&rooms, &players, &sessions)
	if err != nil || rooms != 1 || players != 1 || sessions != 1 {
		t.Fatalf("duplicate start left %d rooms, %d players and %d sessions: %v", rooms, players, sessions, err)
	}
}

func TestSoloRejectsInvalidPersonalitiesBeforeWriting(t *testing.T) {
	handler := New(nil, t.TempDir(), true)
	for _, invalid := range []string{`"expert"`, `""`, `null`, `4`, `true`, `{}`, `[]`} {
		body := `{"id":"c31b16ac-f8fa-40c5-a82f-5a9ceebdadf1","level":` + invalid + `}`
		got := sendRequest(handler, "POST", "/api/solo-games", body, nil)
		if got.Code != http.StatusBadRequest || len(got.Result().Cookies()) != 0 {
			t.Errorf("level %s: %d %s", invalid, got.Code, got.Body)
		}
	}
}

func TestGuestRecoversLostSoloResponse(t *testing.T) {
	pool := openTestDB(t, dbtest.IsolatedDSN(t))
	handler := New(pool, t.TempDir(), true)
	const sessionPath = "/api/session/guest"
	const soloPath = "/api/solo-games"
	const id = "e6cbdaf2-bf1a-4855-a8c2-f85d2474e10a"
	const body = `{"id":"` + id + `","level":"beginner"}`
	guest := sendRequest(handler, "POST", sessionPath, "{}", nil)
	cookies := guest.Result().Cookies()
	if guest.Code != http.StatusCreated || len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatal("fresh guest did not receive a protected cookie before starting")
	}
	host := cookies[0]
	var roomCount int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM rooms").Scan(&roomCount); err != nil || roomCount != 0 {
		t.Fatalf("guest setup created a room: %d, %v", roomCount, err)
	}
	// The client receives nothing from this request, including its headers.
	sendRequest(handler, "POST", soloPath, body, host)
	resumed := sendRequest(handler, "POST", sessionPath, "{}", host)
	if resumed.Code != http.StatusOK || resumed.Body.String() != guest.Body.String() || len(resumed.Result().Cookies()) != 0 {
		t.Fatal("guest setup replaced the original identity on retry")
	}
	retry := sendRequest(handler, "POST", soloPath, `{"id":"`+id+`","level":"master"}`, host)
	room := readGameRoom(t, retry)
	if room.ID != id || room.Listed || room.YourSeat != 1 || room.Game == nil || len(room.Seats) != 2 ||
		room.Seats[1].BotLevel != game.BotBeginner || len(retry.Result().Cookies()) != 0 {
		t.Fatalf("could not recover the original private Beginner game: %+v", room)
	}
	visitor := sendRequest(handler, "POST", sessionPath, "{}", nil)
	if visitor.Code != http.StatusCreated {
		t.Fatalf("create second guest: %d %s", visitor.Code, visitor.Body)
	}
	foreign := sendRequest(handler, "POST", soloPath, body, visitor.Result().Cookies()[0])
	if foreign.Code != http.StatusConflict || len(foreign.Result().Cookies()) != 0 {
		t.Fatalf("another guest recovered the host's private game: %d", foreign.Code)
	}
}

func TestConcurrentSoloSelectionsKeepOneGame(t *testing.T) {
	pool := openTestDB(t, dbtest.IsolatedDSN(t))
	handler := New(pool, t.TempDir(), true)
	guest := sendRequest(handler, "POST", "/api/session", `{"displayName":"Robin"}`, nil)
	if guest.Code != http.StatusCreated {
		t.Fatalf("create guest: %d %s", guest.Code, guest.Body)
	}
	host := guest.Result().Cookies()[0]
	const id = "e6cbdaf2-bf1a-4855-a8c2-f85d2474e10a"
	results := make(chan *httptest.ResponseRecorder, 2)
	for _, level := range []string{"beginner", "competitive"} {
		body := `{"id":"` + id + `","level":"` + level + `"}`
		go func() { results <- sendRequest(handler, "POST", "/api/solo-games", body, host) }()
	}
	first := readGameRoom(t, <-results)
	second := readGameRoom(t, <-results)
	if first.ID != id || len(first.Seats) != 2 || !reflect.DeepEqual(first, second) {
		t.Fatal("concurrent selections returned different games or seats")
	}
	if level := first.Seats[1].BotLevel; level != game.BotBeginner && level != game.BotCompetitive {
		t.Fatalf("concurrent selections used an unrequested level: %q", level)
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
	if room := readGameRoom(t, created); room.Seats[1].BotLevel != game.BotNoob {
		t.Fatalf("legacy request without level started against %q", room.Seats[1].BotLevel)
	}
}
