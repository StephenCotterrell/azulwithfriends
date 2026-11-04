package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/StephenCotterrell/azulwithfriends/internal/database"
	"github.com/StephenCotterrell/azulwithfriends/internal/httpapp"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		slog.Error("server command failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	migrate := len(args) == 1 && args[0] == "migrate"
	if len(args) > 0 && !migrate {
		return errors.New("usage: server [migrate]")
	}
	dbEnv := "DATABASE_URL"
	if migrate {
		dbEnv = "DATABASE_URL_UNPOOLED"
	}
	url := os.Getenv(dbEnv)
	if url == "" {
		return fmt.Errorf("%s is required", dbEnv)
	}
	pool, err := database.Open(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if migrate {
		if err := database.Migrate(startup, pool); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
		slog.Info("database migrations complete")
		return nil
	}
	if err := database.CheckSchema(startup, pool); err != nil {
		return fmt.Errorf("verify database schema (run server migrate before starting): %w", err)
	}
	cancel()

	dist := os.Getenv("WEB_DIST_DIR")
	if dist == "" {
		dist = "web/dist"
	}
	index, err := os.Stat(filepath.Join(dist, "index.html"))
	if err != nil || !index.Mode().IsRegular() {
		return fmt.Errorf("frontend build missing: expected %s/index.html", dist)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("", port))
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           httpapp.New(pool, dist, os.Getenv("ALLOW_INSECURE_COOKIES") != "true"),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 * 1024,
	}
	slog.Info("serving frontend and API", "port", port)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(drain)
	}
}
