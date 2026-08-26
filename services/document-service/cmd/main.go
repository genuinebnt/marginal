// Command document-service is the Track 1 MVP's document service: owns
// pages and blocks, and (later) their outbox. See docs/architecture/DATA_MODEL.md
// and docs/architecture/lld/document-service.md for the full contract.
//
// gRPC (PageService) on :9001, HTTP health probes on :8001 — two listeners,
// per ADR-007: gRPC is the only traffic surface, HTTP never grows a
// business endpoint. See docs/porting/PROGRESS.md for current status.
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	documentv1 "marginal/document-service/internal/genproto/documentv1"
	"marginal/document-service/internal/migrate"
	"marginal/document-service/internal/pages"
)

func main() {
	if err := run(); err != nil {
		slog.Error("document-service failed", "err", err)
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

	grpcAddr := envOr("DOCUMENT_SERVICE_GRPC_ADDR", ":9001")
	httpAddr := envOr("DOCUMENT_SERVICE_HTTP_ADDR", ":8001")

	grpcServer := grpc.NewServer()
	documentv1.RegisterPageServiceServer(grpcServer, &pages.Server{Repo: pages.NewPostgresRepo(pool)})
	reflection.Register(grpcServer) // lets grpcurl/grpcui introspect without a .proto file, local dev only

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}

	httpServer := &http.Server{Addr: httpAddr, Handler: healthMux()}

	errCh := make(chan error, 2)
	go func() {
		slog.Info("document-service gRPC listening", "addr", grpcAddr)
		errCh <- grpcServer.Serve(lis)
	}()
	go func() {
		slog.Info("document-service health listening", "addr", httpAddr)
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

func healthMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
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
