// Package dbtest provides isolated PostgreSQL schemas for integration tests.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/jackc/pgx/v5"
)

func IsolatedDSN(t *testing.T) string {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("set TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	admin, err := database.Open(t.Context(), adminURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var suffix [8]byte
	rand.Read(suffix[:])
	schema := pgx.Identifier{"azul_test_" + hex.EncodeToString(suffix[:])}
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+schema.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// testing cancels t.Context() before calling cleanup functions.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema.Sanitize()+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	dsn, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal("TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	params := dsn.Query()
	params.Set("search_path", schema[0])
	dsn.RawQuery = params.Encode()
	return dsn.String()
}
