package httpapp

import (
	"context"
	"net/http"
	"time"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/game"
	"github.com/jackc/pgx/v5"
)

func (a *app) startGame(w http.ResponseWriter, r *http.Request) {
	if !a.readEmptyObject(w, r) {
		return
	}
	id := r.PathValue("id")
	if !roomIDPattern.MatchString(id) {
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
	if !roomIDPattern.MatchString(id) {
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
