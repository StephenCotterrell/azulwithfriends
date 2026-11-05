package database_test

import (
	"crypto/rand"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/database/dbtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := database.Open(t.Context(), dbtest.IsolatedDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func guestSession() database.RoomSession {
	hash := make([]byte, 32)
	rand.Read(hash)
	return database.RoomSession{NewHash: hash, ExpiresAt: time.Now().Add(time.Hour)}
}

func TestUpgradePreservesExistingGuest(t *testing.T) {
	ctx := t.Context()
	pool := testPool(t)
	v1, err := os.ReadFile("migrations/001_guest_sessions.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(v1), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "CREATE TABLE schema_migrations(version integer PRIMARY KEY); INSERT INTO schema_migrations VALUES (1)", pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	session := guestSession()
	before, err := database.CreatePlayer(ctx, pool, session.NewHash, "Robin", session.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	// Seed an invitation from the version that shipped two-seat waiting rooms.
	v2, err := os.ReadFile("migrations/002_rooms.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(v2), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	var oldRoomID string
	if err := pool.QueryRow(ctx, "INSERT INTO rooms DEFAULT VALUES RETURNING id::text").Scan(&oldRoomID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO room_seats(room_id, number, player_id) VALUES ($1, 1, $2)", oldRoomID, before.ID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := database.Migrate(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	session.ExistingHash = session.NewHash
	after, err := database.FindPlayer(ctx, pool, session.ExistingHash)
	if err != nil || after != before {
		t.Fatalf("guest after upgrade = %+v, %v; want %+v", after, err, before)
	}
	existing, err := database.GetRoom(ctx, pool, oldRoomID, session.ExistingHash)
	if err != nil || existing.Game != nil || existing.YourSeat != 1 || existing.Seats[0].DisplayName != "Robin" {
		t.Fatalf("old invitation after upgrade = %+v, error %v", existing, err)
	}
	if _, _, err := database.JoinRoom(ctx, pool, oldRoomID, guestSession(), false); err != nil {
		t.Fatal(err)
	}
	started, err := database.StartGame(ctx, pool, oldRoomID, session.ExistingHash)
	if err != nil || started.Game == nil || started.Game.Revision != 1 {
		t.Fatalf("start game in an existing room: %+v, %v", started, err)
	}
	room, created, err := database.CreateRoom(ctx, pool, session, false)
	if err != nil || created || room.YourSeat != 1 || room.Seats[0].DisplayName != "Robin" {
		t.Fatalf("create with original session = %+v, new session %t, %v", room, created, err)
	}
}

func TestConcurrentClaimsLeaveTwoPlayersAndTwoSeats(t *testing.T) {
	ctx := t.Context()
	pool := testPool(t)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	invalid := guestSession()
	invalid.NewHash = []byte{0}
	if _, _, err := database.CreateRoom(ctx, pool, invalid, false); err == nil {
		t.Fatal("expected the session constraint to roll back an incomplete room")
	}
	host, created, err := database.CreateRoom(ctx, pool, guestSession(), false)
	if err != nil || !created || host.YourSeat != 1 || host.Seats[0].DisplayName != "Player 1" {
		t.Fatalf("create = %+v, new session %t, %v", host, created, err)
	}
	type claim struct {
		session database.RoomSession
		room    database.Room
		created bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan claim, 2)
	for range 2 {
		go func() {
			session := guestSession()
			<-start
			room, created, err := database.JoinRoom(ctx, pool, host.ID, session, false)
			results <- claim{session, room, created, err}
		}()
	}
	close(start)
	var winner database.RoomSession
	for range 2 {
		result := <-results
		if errors.Is(result.err, database.ErrRoomFull) && !result.created {
			continue
		}
		if result.err != nil || !result.created || result.room.YourSeat != 2 || result.room.Seats[1].DisplayName != "Player 2" || winner.NewHash != nil {
			t.Fatalf("unexpected concurrent join result: room %+v, new session %t, %v", result.room, result.created, result.err)
		}
		winner = result.session
	}
	if winner.NewHash == nil {
		t.Fatal("neither concurrent request took the open seat")
	}
	retry := guestSession()
	retry.ExistingHash = winner.NewHash
	room, created, err := database.JoinRoom(ctx, pool, host.ID, retry, false)
	if err != nil || created || room.YourSeat != 2 {
		t.Fatalf("repeat join = %+v, new session %t, %v", room, created, err)
	}
	if _, _, err := database.JoinRoom(ctx, pool, "00000000-0000-4000-8000-000000000000", guestSession(), false); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("unknown room: %v", err)
	}
	var rooms, seats, players, sessions int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM rooms), (SELECT count(*) FROM room_seats),
		       (SELECT count(*) FROM players), (SELECT count(*) FROM sessions)`).Scan(&rooms, &seats, &players, &sessions); err != nil {
		t.Fatal(err)
	}
	if rooms != 1 || seats != 2 || players != 2 || sessions != 2 {
		t.Fatalf("unexpected writes: %d rooms, %d seats, %d players, %d sessions", rooms, seats, players, sessions)
	}
}
