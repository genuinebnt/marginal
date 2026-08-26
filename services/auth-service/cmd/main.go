// Command auth-service is the Track 1 MVP's auth service: Argon2id
// password hashing, RS256 access tokens, refresh-token rotation, users
// and roles. See docs/architecture/DATA_MODEL.md for the schema.
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

	addr := ":8006"
	slog.Info("auth-service listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
