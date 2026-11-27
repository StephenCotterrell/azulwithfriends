package database_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/game"
)

func TestHistoryAcrossWriterVersions(t *testing.T) {
	ctx := t.Context()
	pool := testPool(t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	seats := [2]database.RoomSession{guestSession(), guestSession()}
	room, _, err := database.CreateRoom(ctx, pool, seats[0], false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.JoinRoom(ctx, pool, room.ID, seats[1], false); err != nil {
		t.Fatal(err)
	}
	room, err = database.StartGame(ctx, pool, room.ID, seats[0].NewHash)
	if err != nil {
		t.Fatal(err)
	}
	var oldWriter game.State
	if err := pool.QueryRow(ctx, "SELECT game_state FROM rooms WHERE id = $1", room.ID).Scan(&oldWriter); err != nil {
		t.Fatal(err)
	}
	if err := oldWriter.Apply(oldWriter.Turn, room.Game.LegalMoves[0]); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(oldWriter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE rooms SET game_revision = 2, game_state = $2::jsonb WHERE id = $1", room.ID, string(encoded)); err != nil {
		t.Fatal(err)
	}
	gap, err := database.GetHistory(ctx, pool, room.ID, seats[1].NewHash)
	if err != nil {
		t.Fatal(err)
	}
	if gap.Revision != 2 || gap.Complete || len(gap.Events) != 2 || gap.Events[0].Kind != "opening" ||
		gap.Events[1].Kind != "baseline" || !reflect.DeepEqual(gap.Events[1].Table, oldWriter.PublicTable()) {
		t.Fatal("the old writer's move should appear as a gap with its actual committed board")
	}

	// Fail the new writer's history insert after it updates the board. Neither
	// change can survive, and the same valid draft must succeed on retry.
	if _, err := pool.Exec(ctx, "ALTER TABLE room_history ADD CONSTRAINT test_block_history CHECK (revision < 3)"); err != nil {
		t.Fatal(err)
	}
	hash := seats[oldWriter.Turn-1].NewHash
	move := oldWriter.View().LegalMoves[0]
	if _, err := database.PlayMove(ctx, pool, room.ID, hash, 2, move); err == nil {
		t.Fatal("expected history write to fail")
	}
	still, err := database.GetHistory(ctx, pool, room.ID, hash)
	if err != nil || !reflect.DeepEqual(still, gap) {
		t.Fatalf("history failure advanced the game: %v", err)
	}
	if _, err := pool.Exec(ctx, "ALTER TABLE room_history DROP CONSTRAINT test_block_history"); err != nil {
		t.Fatal(err)
	}
	room, err = database.PlayMove(ctx, pool, room.ID, hash, 2, move)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := database.GetHistory(ctx, pool, room.Slug, hash)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.MatchID != gap.MatchID || resumed.Complete || resumed.Revision != 3 || len(resumed.Events) != 3 ||
		!reflect.DeepEqual(resumed.Events[:2], gap.Events) || resumed.Events[2].Kind != "draft" ||
		!reflect.DeepEqual(resumed.Events[2].Table, room.Game.Table) {
		t.Fatal("recording failed to resume after the gap")
	}
}
