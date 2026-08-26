// Command auth-service is the Track 1 MVP's auth service: Argon2id
// password hashing, RS256 access tokens, refresh-token rotation, users.
// See docs/api/auth.md for the contract and
// docs/architecture/lld/auth-service.md for the full design.
//
// gRPC (AuthService) on :9006, HTTP on :8006 for health probes and the
// one public route, GET /.well-known/jwks.json (ADR-007: gRPC is the only
// traffic surface otherwise). See docs/porting/PROGRESS.md for status.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"marginal/auth-service/internal/api"
	"marginal/auth-service/internal/authservice"
	"marginal/auth-service/internal/blocklist"
	authv1 "marginal/auth-service/internal/genproto/authv1"
	"marginal/auth-service/internal/keys"
	"marginal/auth-service/internal/lockout"
	"marginal/auth-service/internal/migrate"
	"marginal/auth-service/internal/passwordhash"
)

func main() {
	if err := run(); err != nil {
		slog.Error("auth-service failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errRequiredEnv("DATABASE_URL")
	}
	redisAddr := envOr("REDIS_ADDR", "localhost:6379")

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

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer func() { _ = rdb.Close() }()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	keyStore, err := keys.NewInMemoryStore()
	if err != nil {
		return err
	}
	dummy, err := passwordhash.NewDummy(passwordhash.DefaultParams)
	if err != nil {
		return err
	}

	logger := slog.Default()
	svc := authservice.New(
		pool, keyStore, passwordhash.DefaultParams, dummy,
		blocklist.New(rdb), lockout.New(rdb), logger,
	)

	grpcAddr := envOr("AUTH_SERVICE_GRPC_ADDR", ":9006")
	httpAddr := envOr("AUTH_SERVICE_HTTP_ADDR", ":8006")

	grpcServer := grpc.NewServer()
	authv1.RegisterAuthServiceServer(grpcServer, &api.Server{Service: svc})
	reflection.Register(grpcServer) // local dev only, same as document-service

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}

	httpServer := &http.Server{Addr: httpAddr, Handler: httpMux(keyStore)}

	errCh := make(chan error, 2)
	go func() {
		slog.Info("auth-service gRPC listening", "addr", grpcAddr)
		errCh <- grpcServer.Serve(lis)
	}()
	go func() {
		slog.Info("auth-service HTTP listening", "addr", httpAddr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		grpcServer.GracefulStop()
		return httpServer.Shutdown(context.Background())
	case err := <-errCh:
		grpcServer.GracefulStop()
		return err
	}
}

func httpMux(keyStore keys.Store) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keyStore.JWKS())
	})
	return mux
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type errRequiredEnv string

func (e errRequiredEnv) Error() string { return "missing required env var: " + string(e) }
