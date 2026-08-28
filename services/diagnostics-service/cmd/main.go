// Command diagnostics-service is v2.3.0's analyzer engine (RFC-003) and
// fact dependency graph (ROADMAP.md § Fact dependency graph) — see
// docs/api/diagnostics.md. Stateless: no database of its own, no NATS
// subscription. It reads document-service's PageService/GraphService as
// a gRPC client and computes fresh per request.
//
// gRPC (DiagnosticsService) on :9008, HTTP health probe on :8008 — same
// two-listener shape as every other service, per ADR-007: gRPC is the
// only traffic surface, HTTP never grows a business endpoint.
package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"marginal/envconfig"

	documentv1 "marginal/document-service/genproto/documentv1"

	diagnosticsv1 "marginal/diagnostics-service/genproto/diagnosticsv1"
	"marginal/diagnostics-service/internal/service"
)

func main() {
	if err := run(); err != nil {
		slog.Error("diagnostics-service failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	documentAddr := envconfig.EnvOr("DOCUMENT_SERVICE_GRPC_ADDR", "localhost:9001")

	// insecure.NewCredentials(): east-west traffic within this repo's
	// deployment topology (ADR-007), same as api-gateway's own document-service
	// client — TLS termination, if any, happens at the network edge.
	documentConn, err := grpc.NewClient(documentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = documentConn.Close() }()

	srv := service.NewServer(
		documentv1.NewPageServiceClient(documentConn),
		documentv1.NewGraphServiceClient(documentConn),
	)

	grpcAddr := envconfig.EnvOr("DIAGNOSTICS_SERVICE_GRPC_ADDR", ":9008")
	httpAddr := envconfig.EnvOr("DIAGNOSTICS_SERVICE_HTTP_ADDR", ":8008")

	grpcServer := grpc.NewServer()
	diagnosticsv1.RegisterDiagnosticsServiceServer(grpcServer, srv)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}

	httpServer := &http.Server{Addr: httpAddr, Handler: healthMux()}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 2)
	go func() {
		slog.Info("diagnostics-service gRPC listening", "addr", grpcAddr, "document_service", documentAddr)
		errCh <- grpcServer.Serve(lis)
	}()
	go func() {
		slog.Info("diagnostics-service health listening", "addr", httpAddr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		grpcServer.GracefulStop()
		return httpServer.Close()
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
