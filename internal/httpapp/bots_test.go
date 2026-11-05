package httpapp

import (
	"net/http"
	"reflect"
	"strings"
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
	if got := sendRequest(handler, "POST", url+"/close", "{}", host); got.Code != http.StatusOK {
		t.Fatalf("closing the finished game: %d %s", got.Code, got.Body)
	}
}

func TestCloseActiveBotGame(t *testing.T) {
	pool := openTestDB(t, dbtest.IsolatedDSN(t))
	handler := New(pool, t.TempDir(), true)
	created := sendRequest(handler, "POST", "/api/rooms", "{}", nil)
	room := readGameRoom(t, created)
	host := created.Result().Cookies()[0]
	url := "/api/rooms/" + room.ID
	closeURL := url + "/close"
	for _, withBot := range []bool{false, true} {
		if withBot {
			readGameRoom(t, sendRequest(handler, "POST", url+"/bot", `{"enabled":true}`, host))
		}
		if got := sendRequest(handler, "POST", closeURL, "{}", host); got.Code != http.StatusConflict {
			t.Fatalf("closing a waiting room: %d %s", got.Code, got.Body)
		}
	}
	position := readGameRoom(t, sendRequest(handler, "POST", url+"/start", "{}", host)).Game
	stranger := sendRequest(handler, "POST", "/api/session", `{"displayName":"Guest"}`, nil).Result().Cookies()[0]
	for _, cookie := range []*http.Cookie{nil, stranger} {
		if got := sendRequest(handler, "POST", closeURL, "{}", cookie); got.Code != http.StatusForbidden {
			t.Fatalf("non-host closing a bot game: %d %s", got.Code, got.Body)
		}
	}
	// A draft already holding the lock can finish first. Once the close
	// commits, neither a waiting draft nor a retry can recreate the room.
	draftBody := moveBody(t, position.Revision, position.LegalMoves[0])
	draftStatus := make(chan int, 1)
	go func() { draftStatus <- sendRequest(handler, "POST", url+"/moves", draftBody, host).Code }()
	if got := sendRequest(handler, "POST", closeURL, "{}", host); got.Code != http.StatusOK || strings.TrimSpace(got.Body.String()) != `{"closed":true}` {
		t.Fatalf("closing active bot game: %d %s", got.Code, got.Body)
	}
	if code := <-draftStatus; code != http.StatusOK && code != http.StatusNotFound {
		t.Fatalf("concurrent draft returned %d", code)
	}
	if got := sendRequest(handler, "POST", closeURL, "{}", host); got.Code != http.StatusNotFound {
		t.Fatalf("retry after closing: %d", got.Code)
	}
	if got := sendRequest(handler, "GET", "/api/rooms/"+room.Slug, "", host); got.Code != http.StatusNotFound {
		t.Fatalf("closed invitation: %d", got.Code)
	}
	var seats int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM room_seats WHERE room_id = $1", room.ID).Scan(&seats); err != nil || seats != 0 {
		t.Fatalf("remaining seats after close: %d, %v", seats, err)
	}
	// The same guest can create a fresh room without receiving a replacement cookie.
	next := sendRequest(handler, "POST", "/api/rooms", "{}", host)
	if readGameRoom(t, next).YourSeat != 1 || len(next.Result().Cookies()) != 0 {
		t.Fatal("closing a room lost the host's guest identity")
	}
}
