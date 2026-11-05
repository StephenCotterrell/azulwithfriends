package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/StephenCotterrell/azulwithfriends/internal/game"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoSeat            = errors.New("Join this room to play.")
	ErrStartRequiresHost = errors.New("Only the room host can start the game.")
	ErrNeedPlayers       = errors.New("Both seats must be filled before starting.")
	ErrNotStarted        = errors.New("Start the game before drafting tiles.")
	ErrStaleMove         = errors.New("The board has changed. Choose your move on the current board.")
	ErrIllegalMove       = errors.New("That move is unavailable. Check whose turn it is and choose a legal move.")
)

// StartGame is safe for the host to repeat, including after the last round.
func StartGame(ctx context.Context, pool *pgxpool.Pool, id string, sessionHash []byte) (Room, error) {
	return changeGame(ctx, pool, id, sessionHash, 0, nil)
}

func PlayMove(ctx context.Context, pool *pgxpool.Pool, id string, sessionHash []byte, revision int, move game.Move) (Room, error) {
	return changeGame(ctx, pool, id, sessionHash, revision, &move)
}

func changeGame(ctx context.Context, pool *pgxpool.Pool, id string, sessionHash []byte, revision int, move *game.Move) (Room, error) {
	if len(sessionHash) == 0 {
		return Room{}, ErrNoSeat
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Room{}, err
	}
	defer tx.Rollback(ctx)

	// The same room lock serializes claiming a seat, starting, and drafting.
	// Read the latest committed snapshot only after acquiring it.
	if err := lockRoom(ctx, tx, id); err != nil {
		return Room{}, err
	}
	snapshot, err := readRoom(ctx, tx, id, sessionHash)
	if err != nil {
		return Room{}, err
	}
	if snapshot.room.YourSeat == 0 {
		return Room{}, ErrNoSeat
	}
	if move == nil {
		if snapshot.room.YourSeat != 1 {
			return Room{}, ErrStartRequiresHost
		}
		if snapshot.state != nil {
			return snapshot.publicRoom(), nil
		}
		if len(snapshot.room.Seats) != roomCapacity {
			return Room{}, ErrNeedPlayers
		}
		state := game.New()
		snapshot.state = &state
	} else {
		if snapshot.state == nil {
			return Room{}, ErrNotStarted
		}
		if revision != snapshot.revision {
			return Room{}, ErrStaleMove
		}
		if err := snapshot.state.Apply(snapshot.room.YourSeat, *move); err != nil {
			return Room{}, ErrIllegalMove
		}
	}
	snapshot.revision++
	// The first player is random, and the first-player marker can give the
	// bot consecutive drafts across rounds. Commit only when the host can
	// act again or the game has ended, so refreshes never need to drive it.
	for snapshot.room.Seats[1].IsBot && snapshot.state.Turn == 2 && !snapshot.state.Finished {
		botMove, ok := snapshot.state.BotMove()
		if !ok {
			return Room{}, errors.New("bot has no move on its turn")
		}
		if err := snapshot.state.Apply(2, botMove); err != nil {
			return Room{}, fmt.Errorf("apply bot move: %w", err)
		}
		snapshot.revision++
	}
	encoded, err := json.Marshal(snapshot.state)
	if err != nil {
		return Room{}, err
	}
	if _, err := tx.Exec(ctx, "UPDATE rooms SET game_state = $2::jsonb, game_revision = $3 WHERE id = $1", snapshot.room.ID, string(encoded), snapshot.revision); err != nil {
		return Room{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Room{}, err
	}
	return snapshot.publicRoom(), nil
}
