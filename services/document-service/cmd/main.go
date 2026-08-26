// Command document-service is the Track 1 MVP's document service: owns
// pages and blocks, and (later) their outbox. See docs/architecture/DATA_MODEL.md
// and docs/architecture/lld/document-service.md for the full contract.
//
// Skeleton only for now — health probe endpoint, no gRPC/business logic yet.
// See docs/porting/PROGRESS.md for current status.
package main

import (
	"log/slog"
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	addr := ":8001"
	slog.Info("document-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
