// Command notification-service consumes auth.user_registered
// (DATA_MODEL.md §10 — the one event topic Track 1 can actually produce)
// over NATS and persists a welcome notification, readable at
// GET /notifications (X-Actor-Id header stand-in, same convention as
// every other service — see internal/notify's package doc comment).
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"

	"marginal/authverify"
	"marginal/envconfig"

	"marginal/notification-service/internal/migrate"
	"marginal/notification-service/internal/notify"
)

func main() {
	if err := run(); err != nil {
		slog.Error("notification-service failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseURL, err := envconfig.RequiredEnv("DATABASE_URL")
	if err != nil {
		return err
	}

	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()
	if err := migrate.Up(sqlDB); err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	natsURL := envconfig.EnvOr("NATS_URL", nats.DefaultURL)
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return err
	}
	defer nc.Close()

	repo := notify.NewPostgresRepo(pool)
	sub, err := notify.Subscribe(nc, repo)
	if err != nil {
		return err
	}
	defer func() { _ = sub.Unsubscribe() }()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	notify.NewHandler(repo, authverify.New(envconfig.EnvOr(
		"AUTH_SERVICE_JWKS_URL", "http://localhost:8006/.well-known/jwks.json"))).Mount(mux)

	addr := envconfig.EnvOr("NOTIFICATION_SERVICE_HTTP_ADDR", ":8007")
	httpServer := &http.Server{Addr: addr, Handler: withCORS(mux)}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("notification-service listening", "addr", addr, "nats", natsURL)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return httpServer.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

// withCORS lets web/ (a different origin — different port, same host in
// local dev) actually call this service; without it every fetch from the
// SPA fails at the browser's own CORS check before this handler ever sees
// it, including the preflight OPTIONS request X-Actor-Id triggers as a
// non-simple header (found by trying it in a real browser — see
// docs/porting/PROGRESS.md). "*" is the safe default at this repo's
// scope: nothing here uses cookies or any other ambient credential a
// wildcard origin could leak (auth is the bearer-less header/query
// stand-in pages.md/auth.md/collaboration.md all document).
func withCORS(next http.Handler) http.Handler {
	origin := envconfig.EnvOr("CORS_ALLOWED_ORIGIN", "*")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		// Authorization, since ADR-013 — without it the browser's preflight
		// rejects every call and the request never leaves the page, which is
		// worse than a 401: there is no response to inspect, so an inbox that
		// is being refused looks identical to one that is empty.
		//
		// X-Actor-Id and X-Actor-Kind are gone: nothing reads them, and
		// advertising a header the server ignores invites a client to keep
		// sending it and believe it means something.
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
