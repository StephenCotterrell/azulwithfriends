package database

import (
	"context"
	"errors"

	"github.com/StephenCotterrell/azulwithfriends/internal/game"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrGameStarted = errors.New("The seats are fixed once the game starts.")
	ErrNeedBotGame = errors.New("Only a started game against a bot can be closed.")
)

// SetRoomBot competes for the same open seat as a guest joining by link or lobby.
// An omitted level keeps a seated bot's level and starts a new bot on Competitive.
func SetRoomBot(ctx context.Context, pool *pgxpool.Pool, key string, sessionHash []byte, enabled bool, level game.BotLevel) (Room, error) {
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
	switch {
	case !enabled && !occupied:
		return room, nil
	case !enabled:
		_, err = tx.Exec(ctx, "DELETE FROM room_seats WHERE room_id = $1 AND number = 2 AND is_bot", room.ID)
	case occupied:
		if level == "" || level == room.Seats[1].BotLevel {
			return room, nil
		}
		_, err = tx.Exec(ctx, "UPDATE room_seats SET bot_level = $2 WHERE room_id = $1 AND number = 2 AND is_bot", room.ID, level)
	default:
		if level == "" {
			level = game.BotCompetitive
		}
		err = insertBotSeat(ctx, tx, room.ID, level)
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

func insertBotSeat(ctx context.Context, tx pgx.Tx, roomID string, level game.BotLevel) error {
	_, err := tx.Exec(ctx, "INSERT INTO room_seats (room_id, number, is_bot, bot_level) VALUES ($1, 2, true, $2)", roomID, level)
	return err
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
