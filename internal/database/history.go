package database

import (
	"context"
	"encoding/json"

	"github.com/StephenCotterrell/azulwithfriends/internal/game"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MatchHistory struct {
	MatchID  string              `json:"matchId"`
	Revision int                 `json:"revision"`
	Complete bool                `json:"complete"`
	Events   []game.HistoryEvent `json:"events"`
}

type historyWrite struct {
	before *game.Table // nil for a new game
	events []game.HistoryEvent
}

// Save just this action's events alongside its committed room revision. If the
// previous action came from a version without recording, preserve its actual
// last known position as a baseline before the next recorded draft.
func (h historyWrite) save(ctx context.Context, tx pgx.Tx, s roomSnapshot, previousRevision int) error {
	baseline := "[]"
	if h.before != nil {
		encoded, err := json.Marshal([]game.HistoryEvent{{Kind: "baseline", Table: *h.before}})
		if err != nil {
			return err
		}
		baseline = string(encoded)
	}
	events, err := json.Marshal(h.events)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO room_history (room_id, revision, events)
		VALUES ($1, $2, CASE WHEN EXISTS (
			SELECT 1 FROM room_history WHERE room_id = $1 AND revision = $3
		) THEN '[]'::jsonb ELSE $4::jsonb END || $5::jsonb)`,
		s.room.ID, s.revision, previousRevision, baseline, string(events))
	return err
}

func GetHistory(ctx context.Context, pool *pgxpool.Pool, key string, sessionHash []byte) (MatchHistory, error) {
	if len(sessionHash) == 0 {
		return MatchHistory{}, ErrNoSeat
	}
	// A concurrent draft or rematch must not change the event stream between
	// reading the authorized position and reading that match's events.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return MatchHistory{}, err
	}
	defer tx.Rollback(ctx)
	s, err := readRoom(ctx, tx, key, sessionHash)
	if err != nil {
		return MatchHistory{}, err
	}
	if s.room.YourSeat == 0 {
		return MatchHistory{}, ErrNoSeat
	}
	if s.state == nil {
		return MatchHistory{}, ErrNotStarted
	}
	rows, err := tx.Query(ctx, `
		SELECT revision, events FROM room_history
		WHERE room_id = $1 AND revision > $2 AND revision <= $3 ORDER BY revision`,
		s.room.ID, s.rematchFromRevision, s.revision)
	if err != nil {
		return MatchHistory{}, err
	}
	defer rows.Close()
	history := MatchHistory{MatchID: s.matchID(), Revision: s.revision, Complete: true}
	var lastRevision int
	for rows.Next() {
		var batch []game.HistoryEvent
		if err := rows.Scan(&lastRevision, &batch); err != nil {
			return MatchHistory{}, err
		}
		if batch[0].Kind == "baseline" {
			history.Complete = false
		}
		history.Events = append(history.Events, batch...)
	}
	if err := rows.Err(); err != nil {
		return MatchHistory{}, err
	}
	if lastRevision != s.revision {
		// An older writer may still be serving moves during deployment. A
		// read never invents the missed drafts or mutates the stored log.
		history.Complete = false
		history.Events = append(history.Events, game.HistoryEvent{Kind: "baseline", Table: s.state.PublicTable()})
	}
	return history, tx.Commit(ctx)
}
