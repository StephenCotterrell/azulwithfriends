package httpapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/database/dbtest"
	"github.com/StephenCotterrell/azulwithfriends/internal/game"
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
			!room.Seats[1].IsBot || room.Seats[1].BotLevel != game.BotCompetitive || room.Seats[1].DisplayName == "" || len(added.Result().Cookies()) > 0 {
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
	for _, level := range []string{"noob", "beginner", "intermediate", "competitive", "tactical", "advanced", "master"} {
		body := `{"enabled":true,"level":"` + level + `"}`
		room = readGameRoom(t, sendRequest(handler, "POST", url+"/bot", body, host))
		if len(room.Seats) != 2 || room.Seats[1].BotLevel != game.BotLevel(level) || room.Seats[0].BotLevel != "" {
			t.Fatalf("setting %s: %+v", level, room.Seats)
		}
	}
	// A retry from an older client keeps the host's chosen level.
	room = readGameRoom(t, sendRequest(handler, "POST", url+"/bot", add, host))
	if room.Seats[1].BotLevel != game.BotMaster {
		t.Fatalf("level after retry: %s", room.Seats[1].BotLevel)
	}
	var players, sessions int
	err := pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM players), (SELECT count(*) FROM sessions)").Scan(&players, &sessions)
	if err != nil || players != 1 || sessions != 1 {
		t.Fatalf("bot room has %d players and %d sessions: %v", players, sessions, err)
	}
	rematchURL := url + "/rematch"
	if got := sendRequest(handler, "POST", rematchURL, `{"revision":1}`, host); got.Code != http.StatusConflict {
		t.Fatalf("rematch before start: %d %s", got.Code, got.Body)
	}
	started := readGameRoom(t, sendRequest(handler, "POST", url+"/start", "{}", host))
	if got := sendRequest(handler, "POST", rematchURL, fmt.Sprintf(`{"revision":%d}`, started.Game.Revision), host); got.Code != http.StatusConflict {
		t.Fatalf("rematch during play: %d %s", got.Code, got.Body)
	}
	if got := sendRequest(handler, "POST", url+"/bot", `{"enabled":true,"level":"noob"}`, host); got.Code != http.StatusConflict {
		t.Fatalf("host changed the bot level during play: %d", got.Code)
	}
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
	body := moveBody(t, restored.Game.Revision, shortRowMove(restored.Game.LegalMoves))
	opening := readGameRoom(t, sendRequest(handler, "POST", url+"/moves", body, host)).Game
	if opening.LastMove == nil || opening.LastMove.Seat != 2 || opening.Revision != restored.Game.Revision+2 {
		t.Fatal("host's opening draft was not followed by a bot draft")
	}
	if retry := sendRequest(handler, "POST", url+"/moves", body, host); retry.Code != http.StatusConflict {
		t.Fatalf("stale move accepted: %d", retry.Code)
	}
	position := finishBotGame(t, handler, url, host, opening)
	rematchBody := fmt.Sprintf(`{"revision":%d}`, position.Revision)
	if got := sendRequest(handler, "POST", rematchURL, rematchBody, nil); got.Code != http.StatusForbidden {
		t.Fatalf("anonymous rematch: %d %s", got.Code, got.Body)
	}
	if got := sendRequest(handler, "POST", rematchURL, "{}", host); got.Code != http.StatusBadRequest {
		t.Fatalf("rematch without a revision: %d %s", got.Code, got.Body)
	}
	if got := sendRequest(handler, "POST", rematchURL, fmt.Sprintf(`{"revision":%d}`, position.Revision-1), host); got.Code != http.StatusConflict {
		t.Fatalf("rematch with a stale revision: %d %s", got.Code, got.Body)
	}
	if got := readGameRoom(t, sendRequest(handler, "GET", url, "", host)); !reflect.DeepEqual(got.Game, position) {
		t.Fatal("rejected rematches changed the finished game")
	}

	results := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { results <- sendRequest(handler, "POST", rematchURL, rematchBody, host) }()
	}
	first := <-results
	rematched := readGameRoom(t, first)
	if !reflect.DeepEqual(rematched, readGameRoom(t, <-results)) {
		t.Fatal("concurrent rematches returned different games")
	}
	if rematched.ID != room.ID || rematched.Slug != room.Slug || rematched.YourSeat != 1 || rematched.Listed ||
		!reflect.DeepEqual(rematched.Seats, room.Seats) || len(first.Result().Cookies()) != 0 {
		t.Fatalf("rematch changed the room or seats: %+v", rematched)
	}
	fresh := rematched.Game
	if fresh == nil || fresh.Revision <= position.Revision || fresh.Finished || fresh.Round != 1 || fresh.Turn != 1 ||
		len(fresh.Winners) != 0 || fresh.LastRound != nil || len(fresh.LegalMoves) == 0 {
		t.Fatalf("rematch did not create a playable first round: %+v", fresh)
	}
	for _, player := range fresh.Players {
		if player.Score != 0 || player.Bonus != (game.Bonus{}) || player.Wall != ([5][5]bool{}) {
			t.Fatalf("rematch kept points or tiles from the previous game: %+v", player)
		}
	}
	openingMove := shortRowMove(fresh.LegalMoves)
	if got := sendRequest(handler, "POST", url+"/moves", moveBody(t, position.Revision, openingMove), host); got.Code != http.StatusConflict {
		t.Fatalf("stale move applied to the rematch: %d %s", got.Code, got.Body)
	}
	advanced := readGameRoom(t, sendRequest(handler, "POST", url+"/moves", moveBody(t, fresh.Revision, openingMove), host))
	if retry := readGameRoom(t, sendRequest(handler, "POST", rematchURL, rematchBody, host)); !reflect.DeepEqual(retry, advanced) {
		t.Fatal("retry reset the rematch after a draft")
	}
	finished := finishBotGame(t, handler, url, host, advanced.Game)
	if retry := readGameRoom(t, sendRequest(handler, "POST", rematchURL, rematchBody, host)); !reflect.DeepEqual(retry.Game, finished) {
		t.Fatal("retry reset the rematch after it finished")
	}
	if got := sendRequest(handler, "POST", url+"/close", "{}", host); got.Code != http.StatusOK {
		t.Fatalf("closing the finished game: %d %s", got.Code, got.Body)
	}
}

func finishBotGame(t *testing.T, handler http.Handler, url string, host *http.Cookie, position *database.RoomGame) *database.RoomGame {
	t.Helper()
	limit := position.Revision + 600
	for !position.Finished {
		if position.Turn != 1 || len(position.LegalMoves) == 0 || position.Revision > limit {
			t.Fatalf("bot game stalled at revision %d, turn %d", position.Revision, position.Turn)
		}
		body := moveBody(t, position.Revision, shortRowMove(position.LegalMoves))
		position = readGameRoom(t, sendRequest(handler, "POST", url+"/moves", body, host)).Game
	}
	if len(position.Winners) == 0 || len(position.LegalMoves) != 0 {
		t.Fatal("incomplete bot game")
	}
	return position
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
