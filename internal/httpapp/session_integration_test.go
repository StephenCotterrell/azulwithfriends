package httpapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/database/dbtest"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openTestDB(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}

func sendRequest(handler http.Handler, method, endpoint, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://game.example"+endpoint, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, r)
	return recorder
}

func TestGuestPersistsAcrossRestart(t *testing.T) {
	dsn := dbtest.IsolatedDSN(t)
	first := openTestDB(t, dsn)
	handler := New(first, t.TempDir(), true)
	send := func(method, endpoint, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		return sendRequest(handler, method, endpoint, body, cookie)
	}
	created := send(http.MethodPost, "/api/session", `{"displayName":"  Robin  "}`, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	var guest playerResponse
	if err := json.Unmarshal(created.Body.Bytes(), &guest); err != nil || guest.Player == nil || guest.Player.ID == "" || guest.Player.DisplayName != "Robin" {
		t.Fatalf("unexpected guest response: %s", created.Body)
	}
	cookies := created.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].MaxAge <= 0 {
		t.Fatal("expected one persistent, protected guest cookie")
	}

	first.Close()
	second := openTestDB(t, dsn)
	handler = New(second, t.TempDir(), true)
	restored := send(http.MethodGet, "/api/me", "", cookies[0])
	if restored.Code != http.StatusOK || restored.Body.String() != created.Body.String() {
		t.Fatalf("restored identity differs: %d %s", restored.Code, restored.Body)
	}
	renamed := send(http.MethodPost, "/api/session", `{"displayName":"Jay"}`, cookies[0])
	var updated playerResponse
	if err := json.Unmarshal(renamed.Body.Bytes(), &updated); err != nil || renamed.Code != http.StatusOK || updated.Player == nil || updated.Player.ID != guest.Player.ID || updated.Player.DisplayName != "Jay" {
		t.Fatalf("rename failed to preserve identity: %d %s", renamed.Code, renamed.Body)
	}
	if _, err := second.Exec(t.Context(), "UPDATE sessions SET expires_at = now() - interval '1 minute'"); err != nil {
		t.Fatal(err)
	}
	expired := send(http.MethodGet, "/api/me", "", cookies[0])
	if expired.Code != http.StatusOK || strings.TrimSpace(expired.Body.String()) != `{"player":null}` {
		t.Fatalf("expired session retained its identity: %d %s", expired.Code, expired.Body)
	}
}
