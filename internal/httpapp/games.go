package httpapp

import (
	"context"
	"net/http"
	"time"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/game"
	"github.com/jackc/pgx/v5"
)

func (a *app) getHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validRoomKey(id) {
		roomError(w, pgx.ErrNoRows)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	history, err := database.GetHistory(ctx, a.db, id, a.sessionHash(r))
	if err != nil {
		roomError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (a *app) createSoloGame(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID string `json:"id"`
	}
	if !a.readJSON(w, r, &input) {
		return
	}
	if !roomIDPattern.MatchString(input.ID) {
		problem(w, http.StatusBadRequest, "Send a valid game request ID.")
		return
	}
	if !a.allowRoomWrite(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	cookie, hash := a.newGuestCookie()
	session := database.RoomSession{ExistingHash: a.sessionHash(r), NewHash: hash, ExpiresAt: cookie.Expires}
	room, newSession, err := database.CreateSoloGame(ctx, a.db, input.ID, session)
	if err != nil {
		roomError(w, err)
		return
	}
	if newSession {
		http.SetCookie(w, cookie)
	}
	w.Header().Set("Location", "/room/"+room.Slug)
	writeJSON(w, http.StatusCreated, room)
}

func (a *app) startGame(w http.ResponseWriter, r *http.Request) {
	if !a.readEmptyObject(w, r) {
		return
	}
	id := r.PathValue("id")
	if !validRoomKey(id) {
		roomError(w, pgx.ErrNoRows)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	room, err := database.StartGame(ctx, a.db, id, a.sessionHash(r))
	if err != nil {
		roomError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, room)
}

func (a *app) rematchGame(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Revision int `json:"revision"`
	}
	if !a.readJSON(w, r, &input) {
		return
	}
	if input.Revision < 1 {
		problem(w, http.StatusBadRequest, "Send the revision of the finished game.")
		return
	}
	id := r.PathValue("id")
	if !validRoomKey(id) {
		roomError(w, pgx.ErrNoRows)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	room, err := database.RematchGame(ctx, a.db, id, a.sessionHash(r), input.Revision)
	if err != nil {
		roomError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, room)
}

func (a *app) closeBotGame(w http.ResponseWriter, r *http.Request) {
	if !a.readEmptyObject(w, r) {
		return
	}
	id := r.PathValue("id")
	if !validRoomKey(id) {
		roomError(w, pgx.ErrNoRows)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := database.CloseBotGame(ctx, a.db, id, a.sessionHash(r)); err != nil {
		roomError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"closed": true})
}

func (a *app) playMove(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Revision int    `json:"revision"`
		Source   *int   `json:"source"`
		Color    string `json:"color"`
		Row      *int   `json:"row"`
	}
	if !a.readJSON(w, r, &input) {
		return
	}
	if input.Revision < 1 || input.Source == nil || input.Row == nil || input.Color == "" {
		problem(w, http.StatusBadRequest, "Send the board revision, source, color and destination row.")
		return
	}
	id := r.PathValue("id")
	if !validRoomKey(id) {
		roomError(w, pgx.ErrNoRows)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	move := game.Move{Source: *input.Source, Color: input.Color, Row: *input.Row}
	room, err := database.PlayMove(ctx, a.db, id, a.sessionHash(r), input.Revision, move)
	if err != nil {
		roomError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, room)
}
