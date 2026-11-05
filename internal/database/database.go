package database

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_guest_sessions.sql
var guestSessionMigration string

//go:embed migrations/002_rooms.sql
var roomMigration string

//go:embed migrations/003_games.sql
var gameMigration string

//go:embed migrations/004_lobby.sql
var lobbyMigration string

//go:embed migrations/005_bot_seats.sql
var botSeatMigration string

//go:embed migrations/006_player_rooms_index.sql
var playerRoomsIndexMigration string

func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL connection settings")
	}
	config.MaxConns = 5
	config.MinConns = 0
	config.MaxConnIdleTime = 2 * time.Minute
	config.ConnConfig.ConnectTimeout = 8 * time.Second
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	return pgxpool.NewWithConfig(ctx, config)
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// The transaction lock also covers the first creation of the version table.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('azulwithfriends.migrate'))"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version integer PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}
	for _, migration := range []struct {
		version int
		sql     string
	}{
		{1, guestSessionMigration},
		{2, roomMigration},
		{3, gameMigration},
		{4, lobbyMigration},
		{5, botSeatMigration},
		{6, playerRoomsIndexMigration},
	} {
		var applied bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", migration.version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		if _, err := tx.Exec(ctx, migration.sql, pgx.QueryExecModeSimpleProtocol); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES ($1)", migration.version); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		SELECT p.id, p.display_name, s.token_hash, s.expires_at, r.id, r.game_revision, r.game_state, r.slug, r.listed_at, seat.number, seat.is_bot
		FROM players p JOIN sessions s ON s.player_id = p.id
		JOIN room_seats seat ON seat.player_id = p.id JOIN rooms r ON r.id = seat.room_id
		LIMIT 0`)
	return err
}

type Player struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// Both the standalone guest flow and the room transaction share these queries.
type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func FindPlayer(ctx context.Context, db rowQuerier, hash []byte) (Player, error) {
	var player Player
	err := db.QueryRow(ctx, `
		SELECT p.id::text, p.display_name
		FROM sessions s JOIN players p ON p.id = s.player_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, hash).Scan(&player.ID, &player.DisplayName)
	return player, err
}

func RenamePlayer(ctx context.Context, pool *pgxpool.Pool, hash []byte, name string) (Player, error) {
	var player Player
	err := pool.QueryRow(ctx, `
		UPDATE players p SET display_name = $2
		FROM sessions s
		WHERE s.player_id = p.id AND s.token_hash = $1 AND s.expires_at > now()
		RETURNING p.id::text, p.display_name`, hash, name).Scan(&player.ID, &player.DisplayName)
	return player, err
}

func CreatePlayer(ctx context.Context, db rowQuerier, hash []byte, name string, expires time.Time) (Player, error) {
	player := Player{DisplayName: name}
	// Both inserts commit atomically, in one round trip to the database.
	err := db.QueryRow(ctx, `
		WITH new_player AS (
			INSERT INTO players(display_name) VALUES ($1) RETURNING id
		)
		INSERT INTO sessions(token_hash, player_id, expires_at)
		SELECT $2, id, $3 FROM new_player
		RETURNING player_id::text`, name, hash, expires).Scan(&player.ID)
	return player, err
}
