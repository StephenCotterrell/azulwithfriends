package httpapp

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/StephenCotterrell/azulwithfriends/internal/database/dbtest"
)

func TestHostCanPlayABotToCompletion(t *testing.T) {
	dsn := dbtest.IsolatedDSN(t)
	pool := openTestDB(t, dsn)
	dist := t.TempDir()
	handler := New(pool, dist, true)
	created := sendRequest(handler, "POST", "/api/rooms", `{"listed":true}`, nil)
	room := readGameRoom(t, created)
	host := created.Result().Cookies()[0]
	url := "/api/rooms/" + room.Slug
	add, remove := `{"enabled":true}`, `{"enabled":false}`
	if got := sendRequest(handler, "POST", url+"/bot", add, nil); got.Code != http.StatusForbidden {
		t.Fatalf("anonymous visitor added a bot: %d", got.Code)
	}
	for range 2 {
		added := sendRequest(handler, "POST", url+"/bot", add, host)
		room = readGameRoom(t, added)
		if room.Listed || room.YourSeat != 1 || len(room.Seats) != 2 || room.Seats[0].IsBot ||
			!room.Seats[1].IsBot || room.Seats[1].DisplayName == "" || len(added.Result().Cookies()) > 0 {
			t.Fatalf("unexpected bot room: %+v", room)
		}
	}
	if got := sendRequest(handler, "POST", url+"/join", "{}", nil); got.Code != http.StatusConflict {
		t.Fatalf("visitor claimed the bot's seat: %d", got.Code)
	}
	room = readGameRoom(t, sendRequest(handler, "POST", url+"/bot", remove, host))
	if !room.Listed || len(room.Seats) != 1 {
		t.Fatalf("removing the bot did not reopen the listed room: %+v", room)
	}
	readGameRoom(t, sendRequest(handler, "POST", url+"/bot", add, host))
	var players, sessions int
	err := pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM players), (SELECT count(*) FROM sessions)").Scan(&players, &sessions)
	if err != nil || players != 1 || sessions != 1 {
		t.Fatalf("bot room has %d players and %d sessions: %v", players, sessions, err)
	}
	started := readGameRoom(t, sendRequest(handler, "POST", url+"/start", "{}", host))
	if got := sendRequest(handler, "POST", url+"/bot", remove, host); got.Code != http.StatusConflict {
		t.Fatalf("host removed the bot during play: %d", got.Code)
	}
	if preview := readGameRoom(t, sendRequest(handler, "GET", url, "", nil)); preview.Game != nil || preview.YourSeat != 0 {
		t.Fatalf("visitor received the private game: %+v", preview)
	}

	pool.Close()
	handler = New(openTestDB(t, dsn), dist, true)
	restored := readGameRoom(t, sendRequest(handler, "GET", url, "", host))
	if !reflect.DeepEqual(started, restored) {
		t.Fatal("bot room changed across reconnect")
	}
	position := restored.Game
	replied := false
	for !position.Finished {
		if position.Turn != 1 || len(position.LegalMoves) == 0 || position.Revision > 600 {
			t.Fatalf("bot game stalled at revision %d, turn %d", position.Revision, position.Turn)
		}
		move := position.LegalMoves[0]
		for _, candidate := range position.LegalMoves {
			if candidate.Row > 0 && (move.Row == 0 || candidate.Row < move.Row) {
				move = candidate
			}
		}
		body := moveBody(t, position.Revision, move)
		next := readGameRoom(t, sendRequest(handler, "POST", url+"/moves", body, host)).Game
		if !replied {
			if next.LastMove == nil || next.LastMove.Seat != 2 || next.Revision != position.Revision+2 {
				t.Fatal("host's opening draft was not followed by a bot draft")
			}
			if retry := sendRequest(handler, "POST", url+"/moves", body, host); retry.Code != http.StatusConflict {
				t.Fatalf("stale move accepted: %d", retry.Code)
			}
			replied = true
		}
		position = next
	}
	if !replied || len(position.Winners) == 0 || len(position.LegalMoves) != 0 {
		t.Fatal("incomplete bot game")
	}
}
