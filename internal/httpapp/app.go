package httpapp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionLifetime = 30 * 24 * time.Hour

type app struct {
	db            *pgxpool.Pool
	secureCookies bool
	cookieName    string
	origins       *http.CrossOriginProtection
	creationMu    sync.Mutex
	windowEnd     time.Time
	creations     int
}

func New(db *pgxpool.Pool, dist string, secureCookies bool) http.Handler {
	a := &app{db: db, secureCookies: secureCookies, cookieName: "__Host-azul_guest", origins: http.NewCrossOriginProtection()}
	if !secureCookies {
		a.cookieName = "azul_guest_local"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /api/session", a.saveSession)
	mux.HandleFunc("GET /api/me", a.me)
	mux.HandleFunc("GET /api/lobby", a.lobby)
	mux.HandleFunc("POST /api/rooms", a.saveRoom)
	mux.HandleFunc("GET /api/rooms/{id}", a.getRoom)
	mux.HandleFunc("POST /api/rooms/{id}/join", a.saveRoom)
	mux.HandleFunc("POST /api/rooms/{id}/listing", a.setRoomListing)
	mux.HandleFunc("POST /api/rooms/{id}/bot", a.setRoomBot)
	mux.HandleFunc("POST /api/rooms/{id}/start", a.startGame)
	mux.HandleFunc("POST /api/rooms/{id}/moves", a.playMove)
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) {
		problem(w, http.StatusNotFound, "Unknown API endpoint.")
	})
	files := staticFiles(http.Dir(dist))
	mux.Handle("GET /room/{id}", files)
	mux.Handle("GET /room/{id}/{$}", files)
	mux.Handle("GET /r/{id}", files)
	mux.Handle("GET /r/{id}/{$}", files)
	mux.Handle("GET /", files)
	return mux
}

func staticFiles(dir http.Dir) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path
		isInvitation := r.PathValue("id") != ""
		if isInvitation {
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		}
		if name == "/" || isInvitation {
			name = "/index.html"
		}
		file, err := dir.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasPrefix(name, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func problem(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

type playerResponse struct {
	Player *database.Player `json:"player"`
}

func (a *app) sessionHash(r *http.Request) []byte {
	cookie, err := r.Cookie(a.cookieName)
	if err != nil || len(cookie.Value) != 43 {
		return nil
	}
	token, err := base64.RawURLEncoding.Strict().DecodeString(cookie.Value)
	if err != nil || len(token) != 32 {
		return nil
	}
	hash := sha256.Sum256(token)
	return hash[:]
}

func databaseError(w http.ResponseWriter, err error) {
	slog.Error("guest session database request failed", "error", err)
	problem(w, http.StatusServiceUnavailable, "Your guest session couldn't be loaded or saved. Please try again.")
}

func (a *app) me(w http.ResponseWriter, r *http.Request) {
	hash := a.sessionHash(r)
	if hash == nil {
		writeJSON(w, http.StatusOK, playerResponse{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	player, err := database.FindPlayer(ctx, a.db, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, playerResponse{})
		return
	}
	if err != nil {
		databaseError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, playerResponse{&player})
}

func (a *app) readJSON(w http.ResponseWriter, r *http.Request, input any) bool {
	if err := a.origins.Check(r); err != nil {
		problem(w, http.StatusForbidden, "Please make this request from the game site.")
		return false
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		problem(w, http.StatusUnsupportedMediaType, "Send a JSON request.")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil || decoder.Decode(new(any)) != io.EOF {
		problem(w, http.StatusBadRequest, "Send one JSON object with the expected fields.")
		return false
	}
	return true
}

func (a *app) saveSession(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DisplayName string `json:"displayName"`
	}
	if !a.readJSON(w, r, &input) {
		return
	}
	name := strings.TrimSpace(input.DisplayName)
	if name == "" || utf8.RuneCountInString(name) > 24 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		problem(w, http.StatusBadRequest, "Choose a name between 1 and 24 characters without control characters.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if hash := a.sessionHash(r); hash != nil {
		player, err := database.RenamePlayer(ctx, a.db, hash, name)
		if err == nil {
			writeJSON(w, http.StatusOK, playerResponse{&player})
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			databaseError(w, err)
			return
		}
	}
	if !a.allowCreation(time.Now()) {
		w.Header().Set("Retry-After", "60")
		problem(w, http.StatusTooManyRequests, "Too many guests are joining. Try again in a minute.")
		return
	}
	cookie, hash := a.newGuestCookie()
	player, err := database.CreatePlayer(ctx, a.db, hash, name, cookie.Expires)
	if err != nil {
		databaseError(w, err)
		return
	}
	http.SetCookie(w, cookie)
	writeJSON(w, http.StatusCreated, playerResponse{&player})
}

func (a *app) newGuestCookie() (*http.Cookie, []byte) {
	token := make([]byte, 32)
	rand.Read(token)
	hash := sha256.Sum256(token)
	cookie := &http.Cookie{
		Name: a.cookieName, Value: base64.RawURLEncoding.EncodeToString(token), Path: "/",
		HttpOnly: true, Secure: a.secureCookies, SameSite: http.SameSiteLaxMode,
		MaxAge: int(sessionLifetime.Seconds()), Expires: time.Now().Add(sessionLifetime).UTC(),
	}
	return cookie, hash[:]
}

// Bound anonymous writes on the single public instance without trusting forwarded IPs.
func (a *app) allowCreation(now time.Time) bool {
	a.creationMu.Lock()
	defer a.creationMu.Unlock()
	if !now.Before(a.windowEnd) {
		a.windowEnd = now.Add(time.Minute)
		a.creations = 0
	}
	if a.creations >= 60 {
		return false
	}
	a.creations++
	return true
}
