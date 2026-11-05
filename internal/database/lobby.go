package database

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotListed = errors.New("This room is no longer in the lobby.")
	ErrNotHost   = errors.New("Only the room creator can change its listing.")
)

// A host can list again after an hour. Expiry affects discovery and lobby
// joins only; the invitation remains usable.
const listingActive = "r.listed_at > statement_timestamp() - interval '1 hour'"

type LobbyRoom struct {
	Slug     string `json:"slug"`
	HostName string `json:"hostName"`
	IsHost   bool   `json:"isHost"`
}

func ListLobby(ctx context.Context, pool *pgxpool.Pool, sessionHash []byte) ([]LobbyRoom, error) {
	rows, err := pool.Query(ctx, `
		SELECT r.slug, p.display_name, s.player_id IS NOT NULL
		FROM rooms r
		JOIN room_seats host ON host.room_id = r.id AND host.number = 1
		JOIN players p ON p.id = host.player_id
		LEFT JOIN sessions s ON s.player_id = p.id AND s.token_hash = $1 AND s.expires_at > now()
		WHERE `+listingActive+` AND r.game_revision = 0
		  AND NOT EXISTS (SELECT 1 FROM room_seats guest WHERE guest.room_id = r.id AND guest.number = 2)
		ORDER BY r.listed_at DESC, r.id LIMIT 50`, sessionHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rooms := []LobbyRoom{}
	for rows.Next() {
		var room LobbyRoom
		var name string
		if err := rows.Scan(&room.Slug, &name, &room.IsHost); err != nil {
			return nil, err
		}
		room.HostName = seatLabel(1, name).DisplayName
		rooms = append(rooms, room)
	}
	return rooms, rows.Err()
}

func SetRoomListing(ctx context.Context, pool *pgxpool.Pool, key string, sessionHash []byte, listed bool) (Room, error) {
	if len(sessionHash) == 0 {
		return Room{}, ErrNotHost
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Room{}, err
	}
	defer tx.Rollback(ctx)
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
	if listed && (len(room.Seats) == roomCapacity || room.Game != nil) {
		return Room{}, ErrRoomFull
	}
	_, err = tx.Exec(ctx, "UPDATE rooms SET listed_at = CASE WHEN $2::boolean THEN statement_timestamp() END WHERE id = $1", room.ID, listed)
	if err != nil {
		return Room{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Room{}, err
	}
	room.Listed = listed
	return room, nil
}
