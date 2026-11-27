package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/StephenCotterrell/azulwithfriends/internal/game"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoSeat             = errors.New("Join this room to play.")
	ErrStartRequiresHost  = errors.New("Only the room host can start the game.")
	ErrNeedPlayers        = errors.New("Both seats must be filled before starting.")
	ErrNotStarted         = errors.New("Start the game before drafting tiles.")
	ErrStaleMove          = errors.New("The board has changed. Choose your move on the current board.")
	ErrIllegalMove        = errors.New("That move is unavailable. Check whose turn it is and choose a legal move.")
	ErrRematchUnavailable = errors.New("A rematch is available after finishing a game against a bot.")
	ErrStaleRematch       = errors.New("The game has changed. Use the latest results to start a rematch.")
	ErrSoloStartConflict  = errors.New("This start request couldn’t be resumed. Select Play the computer to try again.")
)

type gameAction uint8

const (
	start gameAction = iota
	draft
	rematch
)

// CreateSoloGame commits the guest, both seats and a playable board together.
func CreateSoloGame(ctx context.Context, pool *pgxpool.Pool, id string, session RoomSession) (Room, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Room{}, false, err
	}
	defer tx.Rollback(ctx)

	room := Room{ID: id}
	err = tx.QueryRow(ctx, "INSERT INTO rooms (id) VALUES ($1) ON CONFLICT (id) DO NOTHING RETURNING slug", id).Scan(&room.Slug)
	if errors.Is(err, pgx.ErrNoRows) {
		// A retry can recover the board only with the original host's cookie.
		room, err := GetRoom(ctx, tx, id, session.ExistingHash)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Room{}, false, err
		}
		if room.YourSeat != 1 || room.Game == nil || len(room.Seats) != roomCapacity || !room.Seats[1].IsBot {
			return Room{}, false, ErrSoloStartConflict
		}
		return room, false, nil
	}
	if err != nil {
		return Room{}, false, err
	}
	room, newSession, err := seatPlayer(ctx, tx, room, session)
	if err != nil {
		return Room{}, false, err
	}
	if err := insertBotSeat(ctx, tx, id, game.BotNoob); err != nil {
		return Room{}, false, err
	}
	room.Seats = append(room.Seats, seatLabel(2, "", game.BotNoob))
	state := game.New()
	snapshot := roomSnapshot{room: room, state: &state}
	started, err := commitGame(ctx, tx, snapshot, true, historyWrite{})
	return started, newSession, err
}

// StartGame is safe for the host to repeat, including after the last round.
func StartGame(ctx context.Context, pool *pgxpool.Pool, id string, sessionHash []byte) (Room, error) {
	return changeGame(ctx, pool, id, sessionHash, start, 0, game.Move{})
}

func PlayMove(ctx context.Context, pool *pgxpool.Pool, id string, sessionHash []byte, revision int, move game.Move) (Room, error) {
	return changeGame(ctx, pool, id, sessionHash, draft, revision, move)
}

func RematchGame(ctx context.Context, pool *pgxpool.Pool, id string, sessionHash []byte, revision int) (Room, error) {
	return changeGame(ctx, pool, id, sessionHash, rematch, revision, game.Move{})
}

func changeGame(ctx context.Context, pool *pgxpool.Pool, id string, sessionHash []byte, action gameAction, revision int, move game.Move) (Room, error) {
	if len(sessionHash) == 0 {
		return Room{}, ErrNoSeat
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Room{}, err
	}
	defer tx.Rollback(ctx)

	// The same room lock serializes seat claims and game updates.
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
	if action != draft && snapshot.room.YourSeat != 1 {
		return Room{}, ErrStartRequiresHost
	}
	isBotGame := len(snapshot.room.Seats) == roomCapacity && snapshot.room.Seats[1].IsBot
	var history historyWrite
	switch action {
	case start:
		if snapshot.state != nil {
			return snapshot.publicRoom(), nil
		}
		if len(snapshot.room.Seats) != roomCapacity {
			return Room{}, ErrNeedPlayers
		}
	case rematch:
		if !isBotGame || snapshot.state == nil {
			return Room{}, ErrRematchUnavailable
		}
		// An exact retry can resume the rematch even after intervening drafts.
		if revision > 0 && revision == snapshot.rematchFromRevision {
			return snapshot.publicRoom(), nil
		}
		if !snapshot.state.Finished {
			return Room{}, ErrRematchUnavailable
		}
		if revision != snapshot.revision {
			return Room{}, ErrStaleRematch
		}
		if _, err := tx.Exec(ctx, "DELETE FROM room_history WHERE room_id = $1", snapshot.room.ID); err != nil {
			return Room{}, err
		}
		snapshot.rematchFromRevision = revision
	case draft:
		if snapshot.state == nil {
			return Room{}, ErrNotStarted
		}
		if revision != snapshot.revision {
			return Room{}, ErrStaleMove
		}
		before := snapshot.state.PublicTable()
		history.before = &before
		history.events, err = snapshot.state.ApplyRecorded(snapshot.room.YourSeat, move)
		if err != nil {
			return Room{}, ErrIllegalMove
		}
	}
	if action != draft {
		state := game.New()
		snapshot.state = &state
	}
	return commitGame(ctx, tx, snapshot, isBotGame, history)
}

func commitGame(ctx context.Context, tx pgx.Tx, snapshot roomSnapshot, isBotGame bool, history historyWrite) (Room, error) {
	previousRevision := snapshot.revision
	if history.before == nil {
		history.events = []game.HistoryEvent{{Kind: "opening", Table: snapshot.state.PublicTable()}}
	}
	// Revisions are scoped to the room, so a draft from an earlier game stays stale.
	snapshot.revision++
	// The first player is random, and the first-player marker can give the
	// bot consecutive drafts across rounds. Commit only when the host can
	// act again or the game has ended, so refreshes never need to drive it.
	for isBotGame && snapshot.state.Turn == 2 && !snapshot.state.Finished {
		botMove, ok := snapshot.state.BotMoveAtLevel(snapshot.room.Seats[1].BotLevel)
		if !ok {
			return Room{}, errors.New("bot has no move on its turn")
		}
		events, err := snapshot.state.ApplyRecorded(2, botMove)
		if err != nil {
			return Room{}, fmt.Errorf("apply bot move: %w", err)
		}
		history.events = append(history.events, events...)
		snapshot.revision++
	}
	encoded, err := json.Marshal(snapshot.state)
	if err != nil {
		return Room{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE rooms SET game_state = $2::jsonb, game_revision = $3, rematch_from_revision = $4
		WHERE id = $1`, snapshot.room.ID, string(encoded), snapshot.revision, snapshot.rematchFromRevision); err != nil {
		return Room{}, err
	}
	if err := history.save(ctx, tx, snapshot, previousRevision); err != nil {
		return Room{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Room{}, err
	}
	return snapshot.publicRoom(), nil
}
