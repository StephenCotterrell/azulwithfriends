package database_test

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/game"
	"github.com/jackc/pgx/v5"
)

func TestLobbyMigrationKeepsActiveGameAndLegacyWrites(t *testing.T) {
	ctx := t.Context()
	pool := testPool(t)
	for _, file := range []string{"001_guest_sessions.sql", "002_rooms.sql", "003_games.sql"} {
		sql, err := os.ReadFile("migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatal(err)
		}
	}
	_, err := pool.Exec(ctx, "CREATE TABLE schema_migrations(version integer PRIMARY KEY); INSERT INTO schema_migrations VALUES (1), (2), (3)", pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		t.Fatal(err)
	}
	seats := [2]database.RoomSession{guestSession(), guestSession()}
	var id string
	// This SQL uses only the columns available to the old Go binary.
	state, err := json.Marshal(game.New())
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO rooms (game_revision, game_state) VALUES (12, $1::jsonb) RETURNING id::text", string(state)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for i, s := range seats {
		p, err := database.CreatePlayer(ctx, pool, s.NewHash, "", s.ExpiresAt)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO room_seats(room_id, number, player_id) VALUES ($1,$2,$3)", id, i+1, p.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	room, err := database.GetRoom(ctx, pool, id, seats[0].NewHash)
	if err != nil {
		t.Fatal(err)
	}
	slugPattern := regexp.MustCompile(`^[a-z]{1,8}(-[a-z]{1,8}){3}$`)
	if room.Listed || !slugPattern.MatchString(room.Slug) || room.ID != id || room.YourSeat != 1 || room.Game.Revision != 12 {
		t.Fatalf("unexpected migrated room %+v", room)
	}
	moved, err := database.PlayMove(ctx, pool, room.Slug, seats[room.Game.Turn-1].NewHash, 12, room.Game.LegalMoves[0])
	if err != nil || moved.ID != id || moved.Game.Revision != 13 {
		t.Fatalf("move through the new link: %+v, %v", moved, err)
	}
	legacy, err := database.GetRoom(ctx, pool, id, seats[1].NewHash)
	if err != nil || legacy.YourSeat != 2 || legacy.Game.Revision != 13 {
		t.Fatalf("legacy link diverged: %+v, %v", legacy, err)
	}

	var oldWriterSlug string
	if err := pool.QueryRow(ctx, "INSERT INTO rooms DEFAULT VALUES RETURNING slug").Scan(&oldWriterSlug); err != nil {
		t.Fatal(err)
	}
	if !slugPattern.MatchString(oldWriterSlug) || oldWriterSlug == room.Slug {
		t.Fatalf("unexpected default slug %q", oldWriterSlug)
	}
}

func TestLobbyExpiryAndRenewal(t *testing.T) {
	ctx := t.Context()
	pool := testPool(t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	host := guestSession()
	room, _, err := database.CreateRoom(ctx, pool, host, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE rooms SET listed_at = now() - interval '61 minutes' WHERE id = $1", room.ID); err != nil {
		t.Fatal(err)
	}
	view, err := database.GetRoom(ctx, pool, room.Slug, host.NewHash)
	if err != nil || view.Listed {
		t.Fatalf("expired listing: %+v, %v", view, err)
	}
	rows, err := database.ListLobby(ctx, pool, nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("expired lobby: %+v, %v", rows, err)
	}
	if _, _, err := database.JoinRoom(ctx, pool, room.Slug, guestSession(), true); !errors.Is(err, database.ErrNotListed) {
		t.Fatalf("expired lobby join: %v", err)
	}
	if _, err := database.SetRoomListing(ctx, pool, room.Slug, host.NewHash, true); err != nil {
		t.Fatal(err)
	}
	rows, err = database.ListLobby(ctx, pool, host.NewHash)
	if err != nil || len(rows) != 1 || !rows[0].IsHost || rows[0].Slug != room.Slug {
		t.Fatalf("renewed lobby: %+v, %v", rows, err)
	}
	if _, err := database.SetRoomListing(ctx, pool, room.ID, host.NewHash, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.JoinRoom(ctx, pool, room.Slug, guestSession(), false); err != nil {
		t.Fatalf("unlisted direct invitation: %v", err)
	}
}

func TestSlugCollisionRetriesWithFreshWords(t *testing.T) {
	ctx := t.Context()
	pool := testPool(t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Shadow just the allocator's random draws in this private schema and
	// transaction. The already-defined rooms.id default still uses pg_catalog.
	_, err = tx.Exec(ctx, `
		SELECT set_config('search_path', quote_ident(current_schema()) || ', pg_catalog', true);
		CREATE SEQUENCE test_draws;
		CREATE FUNCTION gen_random_uuid() RETURNS uuid LANGUAGE sql VOLATILE AS $$
		    SELECT CASE WHEN nextval('test_draws') <= 2
		        THEN '00000000-0000-4000-8000-000000000000'::uuid
		        ELSE '00010000-0000-4000-8000-000000000000'::uuid END
		$$;`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"abandon-abandon-abandon-abandon", "ability-abandon-abandon-abandon"} {
		var got string
		if err := tx.QueryRow(ctx, "INSERT INTO rooms DEFAULT VALUES RETURNING slug").Scan(&got); err != nil || got != want {
			t.Fatalf("room name = %q, error %v, want %q", got, err, want)
		}
	}
}
