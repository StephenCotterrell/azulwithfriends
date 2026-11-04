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
	room, created, err := database.CreateRoom(ctx, pool, session)
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
	if _, _, err := database.CreateRoom(ctx, pool, invalid); err == nil {
		t.Fatal("expected the session constraint to roll back an incomplete room")
	}
	host, created, err := database.CreateRoom(ctx, pool, guestSession())
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
			room, created, err := database.JoinRoom(ctx, pool, host.ID, session)
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
	room, created, err := database.JoinRoom(ctx, pool, host.ID, retry)
	if err != nil || created || room.YourSeat != 2 {
		t.Fatalf("repeat join = %+v, new session %t, %v", room, created, err)
	}
	if _, _, err := database.JoinRoom(ctx, pool, "00000000-0000-4000-8000-000000000000", guestSession()); !errors.Is(err, pgx.ErrNoRows) {
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
