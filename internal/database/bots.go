package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrGameStarted = errors.New("The seats are fixed once the game starts.")
	ErrNeedBotGame = errors.New("Only a started game against a bot can be closed.")
)

// SetRoomBot competes for the same open seat as a guest joining by link or lobby.
func SetRoomBot(ctx context.Context, pool *pgxpool.Pool, key string, sessionHash []byte, enabled bool) (Room, error) {
	if len(sessionHash) == 0 {
		return Room{}, ErrNotHost
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Room{}, err
	}
	defer tx.Rollback(ctx)
	room, err := lockHostRoom(ctx, tx, key, sessionHash)
	if err != nil {
		return Room{}, err
	}
	if room.Game != nil {
		return Room{}, ErrGameStarted
	}
	occupied := len(room.Seats) == roomCapacity
	if occupied && !room.Seats[1].IsBot {
		return Room{}, ErrRoomFull
	}
	if enabled == occupied {
		return room, nil
	}
	if enabled {
		_, err = tx.Exec(ctx, "INSERT INTO room_seats (room_id, number, is_bot) VALUES ($1, 2, true)", room.ID)
	} else {
		_, err = tx.Exec(ctx, "DELETE FROM room_seats WHERE room_id = $1 AND number = 2 AND is_bot", room.ID)
	}
	if err != nil {
		return Room{}, err
	}
	// Removing a bot also restores an unexpired public listing.
	room, err = GetRoom(ctx, tx, key, sessionHash)
	if err != nil {
		return Room{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Room{}, err
	}
	return room, nil
}

// CloseBotGame permanently removes the room and its seats while preserving
// the host's guest identity. It shares the room lock used by human drafts.
func CloseBotGame(ctx context.Context, pool *pgxpool.Pool, key string, sessionHash []byte) error {
	if len(sessionHash) == 0 {
		return ErrNotHost
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	room, err := lockHostRoom(ctx, tx, key, sessionHash)
	if err != nil {
		return err
	}
	if room.Game == nil || len(room.Seats) != roomCapacity || !room.Seats[1].IsBot {
		return ErrNeedBotGame
	}
	if _, err := tx.Exec(ctx, "DELETE FROM rooms WHERE id = $1", room.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
