// Command document-service is the Track 1 MVP's document service: owns
// pages and blocks, and (later) their outbox. See docs/architecture/DATA_MODEL.md
// and docs/architecture/lld/document-service.md for the full contract.
//
// gRPC (PageService, GraphService, SearchService) on :9001, HTTP health
// probes on :8001 — two listeners, per ADR-007: gRPC is the only traffic
// surface, HTTP never grows a business endpoint. See
// docs/porting/PROGRESS.md for current status.
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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"marginal/envconfig"

	documentv1 "marginal/document-service/genproto/documentv1"
	"marginal/document-service/internal/blockproj"
	"marginal/document-service/internal/discover"
	"marginal/document-service/internal/graph"
	"marginal/document-service/internal/migrate"
	"marginal/document-service/internal/pages"
	"marginal/document-service/internal/search"
	"marginal/document-service/internal/spaceproj"
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

	proj := blockproj.New(pool)
	unsubscribe, err := blockproj.Subscribe(nc, proj)
	if err != nil {
		return err
	}
	defer func() { _ = unsubscribe() }()

	// docs.space_members — which spaces a reader may see (ADR-013 §4).
	//
	// Events keep it current; the reconcile puts a FLOOR under how wrong it
	// can get. Both are needed: core NATS has no redelivery, and it delivers
	// nothing at all to a subscriber that was down when the event was
	// published — which makes a restart exactly when this is most likely to
	// be wrong and least likely to notice.
	authConn, err := grpc.NewClient(
		envconfig.EnvOr("AUTH_SERVICE_GRPC_ADDR", "localhost:9006"),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = authConn.Close() }()

	spaces := spaceproj.NewProjector(pool)
	unsubSpaces, err := spaceproj.Subscribe(nc, spaces)
	if err != nil {
		return err
	}
	defer func() { _ = unsubSpaces() }()
	go spaceproj.RunReconcile(ctx, spaces, spaceproj.NewGRPCSource(authConn), 5*time.Minute)

	grpcAddr := envconfig.EnvOr("DOCUMENT_SERVICE_GRPC_ADDR", ":9001")
	httpAddr := envconfig.EnvOr("DOCUMENT_SERVICE_HTTP_ADDR", ":8001")

	searchServer, err := search.NewServer(ctx, search.NewPostgresRepo(pool))
	if err != nil {
		return err
	}
	searchServer.Start(ctx, search.DefaultRefreshInterval)
	defer searchServer.Stop()

	grpcServer := grpc.NewServer()
	documentv1.RegisterPageServiceServer(grpcServer, pages.NewServer(pages.NewPostgresRepo(pool), spaces))
	graphRepo := graph.NewPostgresRepo(pool)
	documentv1.RegisterGraphServiceServer(grpcServer, graph.NewServer(graphRepo))
	// DiscoverService shares the graph repo: link distance is one of § 09's
	// three signals, and it is the same BFS GraphService already runs over the
	// same table — a second reader of one result, not a second implementation.
	documentv1.RegisterDiscoverServiceServer(grpcServer, discover.NewServer(discover.NewPostgresRepo(pool), graphRepo))
	documentv1.RegisterSearchServiceServer(grpcServer, searchServer)
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
