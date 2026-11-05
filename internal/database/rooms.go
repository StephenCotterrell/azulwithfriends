package database

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/StephenCotterrell/azulwithfriends/internal/game"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const roomCapacity = 2

var ErrRoomFull = errors.New("room is full")

type Room struct {
	ID       string    `json:"id"`
	Slug     string    `json:"slug"`
	Listed   bool      `json:"listed"`
	Seats    []Seat    `json:"seats"`
	YourSeat int       `json:"yourSeat"`
	Game     *RoomGame `json:"game"`
}

type RoomGame struct {
	Revision int `json:"revision"`
	game.View
}

type Seat struct {
	Number      int    `json:"number"`
	DisplayName string `json:"displayName"`
	IsBot       bool   `json:"isBot"`
}

func seatLabel(number int, name string, isBot bool) Seat {
	if isBot {
		name = "Azul Bot"
	} else if name == "" {
		name = "Player " + strconv.Itoa(number)
	}
	return Seat{Number: number, DisplayName: name, IsBot: isBot}
}

// NewHash identifies a candidate session. The caller issues its cookie only
// when CreateRoom or JoinRoom reports that the transaction created a session.
type RoomSession struct {
	ExistingHash []byte
	NewHash      []byte
	ExpiresAt    time.Time
}

type roomReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func GetRoom(ctx context.Context, db roomReader, id string, sessionHash []byte) (Room, error) {
	snapshot, err := readRoom(ctx, db, id, sessionHash)
	if err != nil {
		return Room{}, err
	}
	return snapshot.publicRoom(), nil
}

// The private state is separate from Room, the response sent to a browser.
type roomSnapshot struct {
	room     Room
	state    *game.State
	revision int
}

func (s roomSnapshot) publicRoom() Room {
	if s.state != nil {
		s.room.Game = &RoomGame{Revision: s.revision, View: s.state.View()}
	}
	return s.room
}

// Both lookups use a unique index. Legacy UUIDs have four hyphens, whereas
// the four-word room name has three. HTTP validates keys before calling here.
func roomKeyColumn(key string) string {
	if strings.Count(key, "-") == 4 {
		return "r.id"
	}
	return "r.slug"
}

func readRoom(ctx context.Context, db roomReader, key string, sessionHash []byte) (roomSnapshot, error) {
	rows, err := db.Query(ctx, `
		SELECT r.id::text, r.slug, COALESCE(`+listingActive+`, false),
		       seat.number, COALESCE(p.display_name, ''), seat.is_bot, s.player_id IS NOT NULL, r.game_revision,
		       CASE WHEN s.player_id IS NOT NULL THEN r.game_state END
		FROM room_seats seat
		JOIN rooms r ON r.id = seat.room_id
		LEFT JOIN players p ON p.id = seat.player_id
		LEFT JOIN sessions s ON s.player_id = p.id AND s.token_hash = $2 AND s.expires_at > now()
		WHERE `+roomKeyColumn(key)+` = $1 ORDER BY seat.number`, key, sessionHash)
	if err != nil {
		return roomSnapshot{}, err
	}
	defer rows.Close()
	var snapshot roomSnapshot
	for rows.Next() {
		var number int
		var name string
		var isBot, ownsSeat bool
		var state *game.State
		if err := rows.Scan(&snapshot.room.ID, &snapshot.room.Slug, &snapshot.room.Listed, &number, &name, &isBot, &ownsSeat, &snapshot.revision, &state); err != nil {
			return roomSnapshot{}, err
		}
		snapshot.room.Seats = append(snapshot.room.Seats, seatLabel(number, name, isBot))
		if ownsSeat {
			snapshot.room.YourSeat = number
			snapshot.state = state
		}
	}
	if err := rows.Err(); err != nil {
		return roomSnapshot{}, err
	}
	// Room creation always commits its first seat in the same transaction.
	if len(snapshot.room.Seats) == 0 {
		return roomSnapshot{}, pgx.ErrNoRows
	}
	if len(snapshot.room.Seats) == roomCapacity || snapshot.revision != 0 {
		snapshot.room.Listed = false
	}
	return snapshot, nil
}

func CreateRoom(ctx context.Context, pool *pgxpool.Pool, session RoomSession, listed bool) (Room, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Room{}, false, err
	}
	defer tx.Rollback(ctx)
	room := Room{Listed: listed}
	if err := tx.QueryRow(ctx, `
		INSERT INTO rooms (listed_at) VALUES (CASE WHEN $1::boolean THEN statement_timestamp() END)
		RETURNING id::text, slug`, listed).Scan(&room.ID, &room.Slug); err != nil {
		return Room{}, false, err
	}
	return seatAndCommit(ctx, tx, room, session)
}

func JoinRoom(ctx context.Context, pool *pgxpool.Pool, id string, session RoomSession, fromLobby bool) (Room, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Room{}, false, err
	}
	defer tx.Rollback(ctx)
	// Serialize seat claims. The following SELECT gets a fresh read-committed
	// snapshot after the previous claimant releases this room's row lock.
	if err := lockRoom(ctx, tx, id); err != nil {
		return Room{}, false, err
	}
	room, err := GetRoom(ctx, tx, id, session.ExistingHash)
	if err != nil {
		return Room{}, false, err
	}
	if room.YourSeat != 0 {
		return room, false, nil
	}
	if len(room.Seats) >= roomCapacity {
		return Room{}, false, ErrRoomFull
	}
	if fromLobby && !room.Listed {
		return Room{}, false, ErrNotListed
	}
	return seatAndCommit(ctx, tx, room, session)
}

func lockRoom(ctx context.Context, tx pgx.Tx, id string) error {
	var locked int
	return tx.QueryRow(ctx, "SELECT 1 FROM rooms r WHERE "+roomKeyColumn(id)+" = $1 FOR UPDATE", id).Scan(&locked)
}

func lockHostRoom(ctx context.Context, tx pgx.Tx, key string, sessionHash []byte) (Room, error) {
	if err := lockRoom(ctx, tx, key); err != nil {
		return Room{}, err
	}
	room, err := GetRoom(ctx, tx, key, sessionHash)
	if err != nil {
		return Room{}, err
	}
	if room.YourSeat != 1 {
		return Room{}, ErrNotHost
	}
	return room, nil
}

func seatAndCommit(ctx context.Context, tx pgx.Tx, room Room, session RoomSession) (Room, bool, error) {
	var player Player
	if len(session.ExistingHash) > 0 {
		var err error
		player, err = FindPlayer(ctx, tx, session.ExistingHash)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Room{}, false, err
		}
	}
	newSession := player.ID == ""
	if newSession {
		var err error
		player, err = CreatePlayer(ctx, tx, session.NewHash, "", session.ExpiresAt)
		if err != nil {
			return Room{}, false, err
		}
	}
	room.YourSeat = len(room.Seats) + 1
	if _, err := tx.Exec(ctx, "INSERT INTO room_seats (room_id, number, player_id) VALUES ($1, $2, $3)", room.ID, room.YourSeat, player.ID); err != nil {
		return Room{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Room{}, false, err
	}
	room.Seats = append(room.Seats, seatLabel(room.YourSeat, player.DisplayName, false))
	if len(room.Seats) == roomCapacity {
		room.Listed = false
	}
	return room, newSession, nil
}
