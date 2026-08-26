// Command collaboration-service is the Track 1 MVP's stateful service: an
// in-memory rope + CRDT op log per open document, live cursors, one
// doc-actor per page. See docs/architecture/rfc/RFC-002-operation-model.md
// for the op log and WAL framing this owns.
//
// Skeleton only for now — health probe endpoint, no business logic yet.
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

	addr := ":8002"
	slog.Info("collaboration-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
