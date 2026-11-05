package httpapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/database/dbtest"
	"github.com/StephenCotterrell/azulwithfriends/internal/game"
)

func TestGameThroughHTTP(t *testing.T) {
	dsn := dbtest.IsolatedDSN(t)
	pool := openTestDB(t, dsn)
	dist := t.TempDir()
	handler := New(pool, dist, true)

	readRoom := func(r *httptest.ResponseRecorder) database.Room {
		t.Helper()
		if r.Code != http.StatusOK && r.Code != http.StatusCreated {
			t.Fatalf("room request returned %d: %s", r.Code, r.Body)
		}
		var room database.Room
		decoder := json.NewDecoder(r.Body)
		// Unknown keys here include private bag/discard order or RNG state.
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&room); err != nil {
			t.Fatal(err)
		}
		return room
	}
	moveBody := func(rev int, move game.Move) string {
		t.Helper()
		data, err := json.Marshal(struct {
			Revision int `json:"revision"`
			game.Move
		}{rev, move})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	created := sendRequest(handler, "POST", "/api/rooms", "{}", nil)
	room := readRoom(created)
	url := "/api/rooms/" + room.Slug
	legacyURL := "/api/rooms/" + room.ID
	host := created.Result().Cookies()[0]
	if got := sendRequest(handler, "POST", url+"/start", "{}", host); got.Code != http.StatusConflict {
		t.Fatalf("one-player start: %d", got.Code)
	}
	joined := sendRequest(handler, "POST", url+"/join", "{}", nil)
	readRoom(joined)
	players := [2]*http.Cookie{host, joined.Result().Cookies()[0]}
	stranger := sendRequest(handler, "POST", "/api/rooms", "{}", nil).Result().Cookies()[0]
	if got := sendRequest(handler, "POST", url+"/start", "{}", stranger); got.Code != http.StatusForbidden {
		t.Fatalf("outsider start: %d", got.Code)
	}
	start := make(chan *httptest.ResponseRecorder, 2)
	for _, player := range players {
		go func() { start <- sendRequest(handler, "POST", url+"/start", "{}", player) }()
	}
	room = readRoom(<-start)
	second := readRoom(<-start)
	if room.Game == nil || room.Game.Revision != 1 || !reflect.DeepEqual(room.Game, second.Game) {
		t.Fatal("simultaneous starts must return the same initial position")
	}
	for _, cookie := range []*http.Cookie{nil, stranger} {
		if got := readRoom(sendRequest(handler, "GET", url, "", cookie)); got.Game != nil || got.YourSeat != 0 {
			t.Fatal("unseated visitor could inspect a started game")
		}
	}
	position := room.Game
	if len(position.LegalMoves) == 0 {
		t.Fatal("initial position has no legal moves")
	}
	move := position.LegalMoves[0]
	body := moveBody(position.Revision, move)
	active := players[position.Turn-1]
	if got := sendRequest(handler, "POST", url+"/moves", body, stranger); got.Code != http.StatusForbidden {
		t.Fatalf("stranger's move: %d", got.Code)
	}
	if got := sendRequest(handler, "POST", url+"/moves", body, players[2-position.Turn]); got.Code != http.StatusConflict {
		t.Fatalf("wrong seat's move: %d", got.Code)
	}
	if got := sendRequest(handler, "POST", url+"/moves", moveBody(1, game.Move{Color: "invalid"}), active); got.Code != http.StatusConflict {
		t.Fatalf("illegal move: %d", got.Code)
	}
	// Even if a browser retries before receiving its first reply, apply the
	// draft and advance its revision exactly once.
	results := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { results <- sendRequest(handler, "POST", url+"/moves", body, active) }()
	}
	var conflicts, applied int
	for range 2 {
		reply := <-results
		if reply.Code == http.StatusConflict {
			conflicts++
		} else {
			position = readRoom(reply).Game
			applied++
		}
	}
	if applied != 1 || conflicts != 1 || position.Revision != 2 {
		t.Fatalf("duplicate move applied %d times, %d conflicts, revision %d", applied, conflicts, position.Revision)
	}
	pool.Close()
	handler = New(openTestDB(t, dsn), dist, true)
	for index, cookie := range players {
		restored := readRoom(sendRequest(handler, "GET", legacyURL, "", cookie))
		if restored.YourSeat != index+1 || !reflect.DeepEqual(restored.Game, position) {
			t.Fatal("board or seat changed after reopening the database connection")
		}
	}
	for !position.Finished {
		if position.Revision > 600 || len(position.LegalMoves) == 0 {
			t.Fatal("the strategy of filling short rows stopped making progress")
		}
		move := position.LegalMoves[0]
		for _, candidate := range position.LegalMoves {
			if candidate.Row > 0 && (move.Row == 0 || candidate.Row < move.Row) {
				move = candidate
			}
		}
		next := readRoom(sendRequest(handler, "POST", url+"/moves", moveBody(position.Revision, move), players[position.Turn-1])).Game
		if next.Revision != position.Revision+1 {
			t.Fatal("successful move did not advance the revision")
		}
		position = next
	}
	if len(position.Winners) == 0 || len(position.LegalMoves) != 0 {
		t.Fatal("finished game should identify the winners and have no legal moves")
	}
	if got := sendRequest(handler, "POST", url+"/moves", moveBody(position.Revision, move), host); got.Code != http.StatusConflict {
		t.Fatalf("finished game accepted a move: %d", got.Code)
	}
	if got := readRoom(sendRequest(handler, "POST", url+"/start", "{}", host)); !reflect.DeepEqual(got.Game, position) {
		t.Fatal("a repeated start reset the final scores")
	}
}
