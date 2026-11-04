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

var roomIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func (a *app) getRoom(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !roomIDPattern.MatchString(id) {
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
	var input *struct{}
	if !a.readJSON(w, r, &input) {
		return
	}
	if input == nil {
		problem(w, http.StatusBadRequest, "Send an empty JSON object.")
		return
	}
	id := r.PathValue("id")
	if id != "" && !roomIDPattern.MatchString(id) {
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
		room, newSession, err = database.CreateRoom(ctx, a.db, session)
		status = http.StatusCreated
	} else {
		room, newSession, err = database.JoinRoom(ctx, a.db, id, session)
	}
	if err != nil {
		roomError(w, err)
		return
	}
	if newSession {
		http.SetCookie(w, cookie)
	}
	if status == http.StatusCreated {
		w.Header().Set("Location", "/r/"+room.ID)
	}
	writeJSON(w, status, room)
}

func roomError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		problem(w, http.StatusNotFound, "Room not found. Check the invitation link.")
	case errors.Is(err, database.ErrRoomFull):
		problem(w, http.StatusConflict, "Both seats in this room are taken.")
	default:
		slog.Error("room database request failed", "error", err)
		problem(w, http.StatusServiceUnavailable, "The room is temporarily unavailable. Please try again.")
	}
}
