package httpapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGuestPersistsAcrossRestart(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("set TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	open := func(dsn string) *pgxpool.Pool {
		t.Helper()
		pool, err := database.Open(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		return pool
	}
	admin := open(adminURL)
	defer admin.Close()
	var suffix [8]byte
	rand.Read(suffix[:])
	schema := "guest_test_" + hex.EncodeToString(suffix[:])
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	dsn, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	params := dsn.Query()
	params.Set("search_path", schema)
	dsn.RawQuery = params.Encode()
	first := open(dsn.String())
	defer first.Close()
	if err := database.Migrate(ctx, first); err != nil {
		t.Fatal(err)
	}
	handler := New(first, t.TempDir(), true)
	send := func(method, endpoint, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
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
	second := open(dsn.String())
	defer second.Close()
	if err := database.Migrate(ctx, second); err != nil {
		t.Fatalf("repeat deployment failed: %v", err)
	}
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
	if _, err := second.Exec(ctx, "UPDATE sessions SET expires_at = now() - interval '1 minute'"); err != nil {
		t.Fatal(err)
	}
	expired := send(http.MethodGet, "/api/me", "", cookies[0])
	if expired.Code != http.StatusOK || strings.TrimSpace(expired.Body.String()) != `{"player":null}` {
		t.Fatalf("expired session retained its identity: %d %s", expired.Code, expired.Body)
	}
}
