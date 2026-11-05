package httpapp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/jackc/pgx/v5"
)

var (
	roomIDPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	roomSlugPattern = regexp.MustCompile(`^[a-z]{1,16}(-[a-z]{1,16}){3}$`)
)

func validRoomKey(key string) bool {
	return roomSlugPattern.MatchString(key) || roomIDPattern.MatchString(key)
}

func (a *app) lobby(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	rooms, err := database.ListLobby(ctx, a.db, a.sessionHash(r))
	if err != nil {
		slog.Error("lobby database request failed", "error", err)
		problem(w, http.StatusServiceUnavailable, "The lobby is temporarily unavailable. Please try again.")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Rooms []database.LobbyRoom `json:"rooms"`
	}{rooms})
}

func (a *app) getRoom(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validRoomKey(id) {
		roomError(w, pgx.ErrNoRows)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	room, err := database.GetRoom(ctx, a.db, id, a.sessionHash(r))
	if err != nil {
		roomError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, room)
}

// The create and join routes both claim a seat and issue a guest cookie only
// after the seat and session have been committed together.
func (a *app) saveRoom(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	option := "listed"
	if id != "" {
		option = "fromLobby"
	}
	enabled, ok := a.readBoolOption(w, r, option, false)
	if !ok {
		return
	}
	if id != "" && !validRoomKey(id) {
		roomError(w, pgx.ErrNoRows)
		return
	}
	if !a.allowCreation(time.Now()) {
		w.Header().Set("Retry-After", "60")
		problem(w, http.StatusTooManyRequests, "Too many requests. Try again in a minute.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cookie, hash := a.newGuestCookie()
	session := database.RoomSession{ExistingHash: a.sessionHash(r), NewHash: hash, ExpiresAt: cookie.Expires}
	var room database.Room
	var newSession bool
	var err error
	status := http.StatusOK
	if id == "" {
		room, newSession, err = database.CreateRoom(ctx, a.db, session, enabled)
		status = http.StatusCreated
	} else {
		room, newSession, err = database.JoinRoom(ctx, a.db, id, session, enabled)
	}
	if err != nil {
		roomError(w, err)
		return
	}
	if newSession {
		http.SetCookie(w, cookie)
	}
	if status == http.StatusCreated {
		w.Header().Set("Location", "/room/"+room.Slug)
	}
	writeJSON(w, status, room)
}

func (a *app) setRoomListing(w http.ResponseWriter, r *http.Request) {
	listed, ok := a.readBoolOption(w, r, "listed", true)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validRoomKey(id) {
		roomError(w, pgx.ErrNoRows)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	room, err := database.SetRoomListing(ctx, a.db, id, a.sessionHash(r), listed)
	if err != nil {
		roomError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, room)
}

// Each route takes a single boolean. An omitted option on create/join keeps
// the original link-only behavior. Reject explicit nulls and unrelated keys.
func (a *app) readBoolOption(w http.ResponseWriter, r *http.Request, name string, required bool) (bool, bool) {
	var input map[string]*bool
	if !a.readJSON(w, r, &input) {
		return false, false
	}
	if input != nil && len(input) == 0 && !required {
		return false, true
	}
	value := input[name]
	if value == nil || len(input) != 1 {
		problem(w, http.StatusBadRequest, "Send a JSON object with a boolean "+name+" field.")
		return false, false
	}
	return *value, true
}

func (a *app) readEmptyObject(w http.ResponseWriter, r *http.Request) bool {
	var input *struct{}
	if !a.readJSON(w, r, &input) {
		return false
	}
	if input == nil {
		problem(w, http.StatusBadRequest, "Send an empty JSON object.")
		return false
	}
	return true
}

func roomError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "Room not found. Check the invitation link.")
	case errors.Is(err, database.ErrRoomFull):
		problem(w, http.StatusConflict, "Both seats in this room are taken.")
	case errors.Is(err, database.ErrNoSeat), errors.Is(err, database.ErrNotHost), errors.Is(err, database.ErrStartRequiresHost):
		problem(w, http.StatusForbidden, err.Error())
	case errors.Is(err, database.ErrNotListed), errors.Is(err, database.ErrNeedPlayers), errors.Is(err, database.ErrNotStarted),
		errors.Is(err, database.ErrStaleMove), errors.Is(err, database.ErrIllegalMove):
		problem(w, http.StatusConflict, err.Error())
	default:
		slog.Error("room database request failed", "error", err)
		problem(w, http.StatusServiceUnavailable, "The room is temporarily unavailable. Please try again.")
	}
}
