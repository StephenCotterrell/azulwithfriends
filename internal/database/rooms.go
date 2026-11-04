package database

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const roomCapacity = 2

var ErrRoomFull = errors.New("room is full")

type Room struct {
	ID       string `json:"id"`
	Seats    []Seat `json:"seats"`
	YourSeat int    `json:"yourSeat"`
}

type Seat struct {
	Number      int    `json:"number"`
	DisplayName string `json:"displayName"`
}

func seatLabel(number int, name string) Seat {
	if name == "" {
		name = "Player " + strconv.Itoa(number)
	}
	return Seat{Number: number, DisplayName: name}
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
	rows, err := db.Query(ctx, `
		SELECT seat.number, p.display_name, s.player_id IS NOT NULL
		FROM room_seats seat
		JOIN players p ON p.id = seat.player_id
		LEFT JOIN sessions s ON s.player_id = p.id AND s.token_hash = $2 AND s.expires_at > now()
		WHERE seat.room_id = $1 ORDER BY seat.number`, id, sessionHash)
	if err != nil {
		return Room{}, err
	}
	defer rows.Close()
	room := Room{ID: id}
	for rows.Next() {
		var number int
		var name string
		var ownsSeat bool
		if err := rows.Scan(&number, &name, &ownsSeat); err != nil {
			return Room{}, err
		}
		room.Seats = append(room.Seats, seatLabel(number, name))
		if ownsSeat {
			room.YourSeat = number
		}
	}
	if err := rows.Err(); err != nil {
		return Room{}, err
	}
	// Room creation always commits its first seat in the same transaction.
	if len(room.Seats) == 0 {
		return Room{}, pgx.ErrNoRows
	}
	return room, nil
}

func CreateRoom(ctx context.Context, pool *pgxpool.Pool, session RoomSession) (Room, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Room{}, false, err
	}
	defer tx.Rollback(ctx)
	var room Room
	if err := tx.QueryRow(ctx, "INSERT INTO rooms DEFAULT VALUES RETURNING id::text").Scan(&room.ID); err != nil {
		return Room{}, false, err
	}
	return seatAndCommit(ctx, tx, room, session)
}

func JoinRoom(ctx context.Context, pool *pgxpool.Pool, id string, session RoomSession) (Room, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Room{}, false, err
	}
	defer tx.Rollback(ctx)
	// Serialize seat claims. The following SELECT gets a fresh read-committed
	// snapshot after the previous claimant releases this room's row lock.
	if err := tx.QueryRow(ctx, "SELECT id::text FROM rooms WHERE id = $1 FOR UPDATE", id).Scan(&id); err != nil {
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
	return seatAndCommit(ctx, tx, room, session)
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
	room.Seats = append(room.Seats, seatLabel(room.YourSeat, player.DisplayName))
	return room, newSession, nil
}
