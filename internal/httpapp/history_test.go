package httpapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/database/dbtest"
	"github.com/StephenCotterrell/azulwithfriends/internal/game"
)

type historyResponse struct {
	MatchID  string `json:"matchId"`
	Revision int    `json:"revision"`
	Complete bool   `json:"complete"`
	Events   []struct {
		Kind  string     `json:"kind"`
		Table game.Table `json:"table"`
	} `json:"events"`
}

func getHistory(t *testing.T, handler http.Handler, url string, cookie *http.Cookie, current *database.RoomGame) historyResponse {
	t.Helper()
	reply := sendRequest(handler, "GET", url+"/history", "", cookie)
	if reply.Code != http.StatusOK || reply.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("history request returned %d, cache control %q: %s", reply.Code, reply.Header().Get("Cache-Control"), reply.Body)
	}
	var history historyResponse
	decoder := json.NewDecoder(reply.Body)
	// Reject private state and action controls, including at the snapshot level.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&history); err != nil {
		t.Fatal(err)
	}
	if history.MatchID == "" || history.MatchID != current.MatchID || history.Revision != current.Revision {
		t.Fatalf("history identifies match %q revision %d; live board identifies %q revision %d", history.MatchID, history.Revision, current.MatchID, current.Revision)
	}
	if n := len(history.Events); n == 0 || !reflect.DeepEqual(history.Events[n-1].Table, current.Table) {
		t.Fatal("history did not end at the committed public board")
	}
	return history
}

func TestHistorySharedAcrossSeatsAndReconnect(t *testing.T) {
	dsn := dbtest.IsolatedDSN(t)
	pool := openTestDB(t, dsn)
	dist := t.TempDir()
	handler := New(pool, dist, true)
	created := sendRequest(handler, "POST", "/api/rooms", "{}", nil)
	room := readGameRoom(t, created)
	url := "/api/rooms/" + room.Slug
	host := created.Result().Cookies()[0]
	joined := sendRequest(handler, "POST", url+"/join", "{}", nil)
	readGameRoom(t, joined)
	seats := [2]*http.Cookie{host, joined.Result().Cookies()[0]}
	position := readGameRoom(t, sendRequest(handler, "POST", url+"/start", "{}", host)).Game
	opening := getHistory(t, handler, url, host, position)
	if !opening.Complete || len(opening.Events) != 1 || opening.Events[0].Kind != "opening" {
		t.Fatal("new multiplayer game must start with its exact opening board")
	}
	for range 2 {
		active := seats[position.Turn-1]
		body := moveBody(t, position.Revision, shortRowMove(position.LegalMoves))
		position = readGameRoom(t, sendRequest(handler, "POST", url+"/moves", body, active)).Game
		if retry := sendRequest(handler, "POST", url+"/moves", body, active); retry.Code != http.StatusConflict {
			t.Fatalf("stale draft returned %d", retry.Code)
		}
	}
	history := getHistory(t, handler, url, host, position)
	if !history.Complete || history.MatchID != opening.MatchID || len(history.Events) != 3 || history.Events[1].Kind != "draft" || history.Events[2].Kind != "draft" {
		t.Fatal("accepted drafts and the opening must each appear exactly once")
	}
	if history.Events[1].Table.LastMove.Seat != opening.Events[0].Table.Turn || history.Events[2].Table.LastMove.Seat == history.Events[1].Table.LastMove.Seat {
		t.Fatal("human drafts have the wrong actor or order")
	}
	pool.Close()
	pool = openTestDB(t, dsn)
	handler = New(pool, dist, true)
	if other := getHistory(t, handler, "/api/rooms/"+room.ID, seats[1], position); !reflect.DeepEqual(other, history) {
		t.Fatal("guest seat received different history after reconnecting by room ID")
	}
	outsider := sendRequest(handler, "POST", "/api/session", `{"displayName":"Other"}`, nil).Result().Cookies()[0]
	if _, err := pool.Exec(t.Context(), "UPDATE sessions SET expires_at = now() - interval '1 minute' WHERE player_id = (SELECT player_id FROM room_seats WHERE room_id = $1 AND number = 1)", room.ID); err != nil {
		t.Fatal(err)
	}
	for _, cookie := range []*http.Cookie{nil, outsider, host} {
		reply := sendRequest(handler, "GET", url+"/history", "", cookie)
		if reply.Code != http.StatusForbidden {
			t.Fatalf("unseated or expired session read history: %d", reply.Code)
		}
	}
}

func TestSoloHistoryThroughFinalScoringAndRematch(t *testing.T) {
	pool := openTestDB(t, dbtest.IsolatedDSN(t))
	handler := New(pool, t.TempDir(), true)
	created := sendRequest(handler, "POST", "/api/solo-games", `{"id":"a30d79ff-80e0-4578-8458-cdea27a70103"}`, nil)
	room := readGameRoom(t, created)
	host := created.Result().Cookies()[0]
	url := "/api/rooms/" + room.Slug
	final := finishBotGame(t, handler, url, host, room.Game)
	history := getHistory(t, handler, url, host, final)
	if !history.Complete || history.Events[0].Kind != "opening" || history.Events[0].Table.LastMove != nil {
		t.Fatal("solo game lost its opening before automatic bot play")
	}
	counts := make(map[string]int)
	for i, event := range history.Events {
		counts[event.Kind]++
		switch event.Kind {
		case "opening":
		case "draft":
			if i == 0 || event.Table.LastMove == nil || event.Table.LastMove.Seat != history.Events[i-1].Table.Turn {
				t.Fatalf("draft %d lacks its actor or is out of order", i)
			}
		case "scoring":
			if i == 0 || history.Events[i-1].Kind != "draft" || event.Table.LastRound == nil || event.Table.LastRound.Round != counts["scoring"] {
				t.Fatalf("round scoring %d is missing or out of order", i)
			}
			for seat, delta := range event.Table.LastRound.Players {
				previous := history.Events[i-1].Table.Players[seat].Score
				if event.Table.Players[seat].Score != previous+delta.Wall-delta.Penalty {
					t.Fatalf("event %d, seat %d: round score includes the wrong points", i, seat+1)
				}
			}
		case "refill", "final":
			if i == 0 || history.Events[i-1].Kind != "scoring" {
				t.Fatalf("%s appeared without the round score", event.Kind)
			}
		default:
			t.Fatalf("unexpected event %q in complete match", event.Kind)
		}
	}
	if counts["opening"] != 1 || counts["draft"] != final.Revision-1 || counts["scoring"] != final.Round || counts["refill"] != final.Round-1 || counts["final"] != 1 || history.Events[len(history.Events)-1].Kind != "final" {
		t.Fatalf("missing or duplicate automatic events in %d rounds, %d drafts: %v", final.Round, final.Revision-1, counts)
	}
	body := fmt.Sprintf(`{"revision":%d}`, final.Revision)
	fresh := readGameRoom(t, sendRequest(handler, "POST", url+"/rematch", body, host)).Game
	open := getHistory(t, handler, url, host, fresh)
	if !open.Complete || open.MatchID == history.MatchID || open.Events[0].Kind != "opening" || len(open.Events) != fresh.Revision-final.Revision {
		t.Fatal("rematch retained events or identity from its predecessor")
	}
	readGameRoom(t, sendRequest(handler, "POST", url+"/rematch", body, host))
	if after := getHistory(t, handler, url, host, fresh); !reflect.DeepEqual(after, open) {
		t.Fatal("retrying a rematch recorded an extra opening")
	}
}

func TestHistoryForGamesStartedBeforeRecording(t *testing.T) {
	pool := openTestDB(t, dbtest.IsolatedDSN(t))
	handler := New(pool, t.TempDir(), true)
	for _, finished := range []bool{false, true} {
		created := sendRequest(handler, "POST", "/api/rooms", "{}", nil)
		room := readGameRoom(t, created)
		host := created.Result().Cookies()[0]
		url := "/api/rooms/" + room.Slug
		readGameRoom(t, sendRequest(handler, "POST", url+"/bot", `{"enabled":true,"level":"noob"}`, host))
		// Use only pre-history columns, as an older server would. The live fixture
		// ends the round on the bot's draft and gives it the next opening draft.
		state := game.New()
		state.Round, state.Turn, state.FirstSeat = 3, 1, 2
		state.MarkerInCenter = false
		state.Factories = [5][]string{{"red", "blue"}}
		if finished {
			for turns := 0; !state.Finished; turns++ {
				if turns > 600 {
					t.Fatal("legacy fixture stalled")
				}
				if err := state.Apply(state.Turn, shortRowMove(state.View().LegalMoves)); err != nil {
					t.Fatal(err)
				}
			}
		}
		encoded, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(t.Context(), "UPDATE rooms SET game_state = $1::jsonb, game_revision = 41 WHERE id = $2", string(encoded), room.ID); err != nil {
			t.Fatal(err)
		}
		position := readGameRoom(t, sendRequest(handler, "GET", url, "", host)).Game
		history := getHistory(t, handler, url, host, position)
		if history.Complete || len(history.Events) != 1 || history.Events[0].Kind != "baseline" {
			t.Fatalf("legacy game finished=%t has an invented past", finished)
		}
		if finished {
			continue
		}
		body := moveBody(t, position.Revision, game.Move{Source: 1, Color: "red", Row: 1})
		next := readGameRoom(t, sendRequest(handler, "POST", url+"/moves", body, host)).Game
		continued := getHistory(t, handler, url, host, next)
		want := []string{"baseline", "draft", "draft", "scoring", "refill", "draft"}
		if continued.Complete || continued.MatchID != history.MatchID || len(continued.Events) != len(want) || !reflect.DeepEqual(continued.Events[0], history.Events[0]) {
			t.Fatal("legacy game did not preserve its partial history and initial baseline")
		}
		for i, kind := range want {
			if continued.Events[i].Kind != kind {
				t.Fatalf("event %d: got %s, want %s", i, continued.Events[i].Kind, kind)
			}
		}
		if continued.Events[2].Table.LastMove.Seat != 2 || continued.Events[5].Table.LastMove.Seat != 2 {
			t.Fatal("consecutive automatic bot drafts across a round were lost")
		}
	}
}
