package database_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/game"
)

func TestBotCompetesWithGuestForLastSeat(t *testing.T) {
	ctx := t.Context()
	pool := testPool(t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	host, guest := guestSession(), guestSession()
	room, _, err := database.CreateRoom(ctx, pool, host, true)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	botClaim, guestClaim := make(chan error, 1), make(chan error, 1)
	go func() {
		<-start
		_, err := database.SetRoomBot(ctx, pool, room.ID, host.NewHash, true)
		botClaim <- err
	}()
	go func() {
		<-start
		_, _, err := database.JoinRoom(ctx, pool, room.Slug, guest, true)
		guestClaim <- err
	}()
	close(start)
	botErr, guestErr := <-botClaim, <-guestClaim
	if !(botErr == nil && errors.Is(guestErr, database.ErrRoomFull) ||
		guestErr == nil && errors.Is(botErr, database.ErrRoomFull)) {
		t.Fatalf("expected exactly one successful claim: bot %v, guest %v", botErr, guestErr)
	}
	after, err := database.GetRoom(ctx, pool, room.ID, guest.NewHash)
	if err != nil || len(after.Seats) != 2 || after.Seats[1].IsBot != (botErr == nil) || (after.YourSeat == 2) != (guestErr == nil) {
		t.Fatalf("roster disagrees with winning claim: %+v, %v", after, err)
	}
}

func TestBotClosesOneRoundAndStartsTheNext(t *testing.T) {
	ctx := t.Context()
	pool := testPool(t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	host := guestSession()
	room, _, err := database.CreateRoom(ctx, pool, host, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SetRoomBot(ctx, pool, room.ID, host.NewHash, true); err != nil {
		t.Fatal(err)
	}
	// After the host empties the last factory, the bot must take red and
	// the marker, then open the next round before returning control.
	state := game.State{
		Table: game.Table{
			Round: 1, Turn: 1, MarkerInCenter: true,
			Factories: [5][]string{{"blue", "blue", "blue", "blue"}}, Center: []string{"red"},
		},
		FirstSeat: 1, Bag: []string{"yellow", "red", "blue", "white", "black", "blue", "white", "red"},
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE rooms SET game_state = $2::jsonb, game_revision = 9 WHERE id = $1", room.ID, string(encoded)); err != nil {
		t.Fatal(err)
	}
	moved, err := database.PlayMove(ctx, pool, room.Slug, host.NewHash, 9, game.Move{Source: 1, Color: "blue", Row: 4})
	if err != nil {
		t.Fatal(err)
	}
	board := moved.Game
	if board.Revision != 12 || board.Round != 2 || board.Turn != 1 || board.LastMove == nil || board.LastMove.Seat != 2 {
		t.Fatalf("expected two automatic bot drafts: %+v", board)
	}
}
