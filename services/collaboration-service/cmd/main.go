// Command collaboration-service is the Track 1 MVP's stateful service: an
// in-memory rope + CRDT op log per open document, live cursors, one
// doc-actor per page. See docs/architecture/rfc/RFC-002-operation-model.md
// for the op log and WAL framing this owns, and docs/api/collaboration.md
// for the WebSocket wire contract this serves.
//
// HTTP only — health probes and the WebSocket upgrade at
// /collab/pages/{id} (internal/wsapi). See docs/porting/PROGRESS.md for
// current status.
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"marginal/authverify"
	"marginal/envconfig"

	"marginal/collaboration-service/internal/migrate"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/opstore"
	"marginal/collaboration-service/internal/outbox"
	"marginal/collaboration-service/internal/pageop"
	"marginal/collaboration-service/internal/roles"
	"marginal/collaboration-service/internal/session"
	"marginal/collaboration-service/internal/wsapi"
)

func main() {
	if err := run(); err != nil {
		slog.Error("collaboration-service failed", "err", err)
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

	poller := outbox.NewPoller(pool, nc)
	go poller.Run(ctx, func(err error) {
		slog.Error("collaboration-service: outbox poller", "err", err)
	})

	// A persistent volume in any real deployment — see
	// deploy/terraform/README.md's "known limitations" on what an
	// abrupt (not orderly) loss of this directory costs. A plain local
	// dir is the right default for local dev and this repo's demo scale.
	walDir := envconfig.EnvOr("COLLAB_WAL_DIR", "./data/collab-wal")
	if err := os.MkdirAll(walDir, 0o700); err != nil {
		return err
	}

	// This process's own identity for Lamport ItemIDs (internal/doctext),
	// not an editing user's — and it MUST be stable across restarts, not
	// fresh per process start (a real bug this repo shipped with until
	// found live): anchor.ItemID{Actor, Counter} is embedded in every
	// persisted op that names an anchor (DeleteText.Range, InsertText's
	// own inverse), so a later replay (session.open, Trace, RestoreTo,
	// palimpsest.Build — anything that calls applyReplayedOp) using a
	// DIFFERENT serverActor generates DIFFERENT ItemIDs for the same
	// historical inserts, and every anchor resolution against
	// already-persisted history then fails with "anchor refers to an
	// item this text never saw." A single collaboration-service instance
	// never has two concurrently-running processes racing to assign the
	// SAME actor's ids at once (this is a stateful, one-writer-per-page
	// service — ARCHITECTURE.md), so there is no uniqueness problem a
	// fresh-per-restart value was ever actually solving; there was only
	// a stability problem it was silently causing. Configurable (in case
	// a future multi-instance deploy genuinely needs two collaboration-
	// service processes each writing under their own distinct tag), but
	// the default must be fixed forever, not regenerated.
	serverActor := envconfig.EnvOr("COLLAB_SERVER_ACTOR", "server")

	repo := opstore.NewPostgresRepo(pool)
	// Roles are resolved once per join and read by can_apply on every op
	// (ADR-013 §3). The two gRPC clients are for the JOIN path only —
	// resolving a role needs document-service (which space is this page in)
	// and auth-service (what is this actor in that space).
	documentConn, err := grpc.NewClient(
		envconfig.EnvOr("DOCUMENT_SERVICE_GRPC_ADDR", "localhost:9001"),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = documentConn.Close() }()

	authConn, err := grpc.NewClient(
		envconfig.EnvOr("AUTH_SERVICE_GRPC_ADDR", "localhost:9006"),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = authConn.Close() }()

	directory := roles.New()
	ws := roles.WS{Dir: directory, Resolver: roles.NewResolver(documentConn, authConn)}

	// can_apply reads the directory and nothing else — RFC-002 §5's single
	// auditable chokepoint, still synchronous and still pure. Every writer
	// arrived through a join that resolved a role first, so an absent entry
	// means an expired one or a path that skipped the join, and both are
	// refused rather than trusted.
	manager := session.NewManager(repo, walDir, serverActor,
		session.WithCanApply(func(pageID uuid.UUID, _ pageop.Op, actorID uuid.UUID, _ oplog.ActorKind) bool {
			return directory.CanApply(pageID, actorID)
		}))
	defer func() {
		if err := manager.CloseAll(); err != nil {
			slog.Error("closing sessions during shutdown", "err", err)
		}
	}()

	httpAddr := envconfig.EnvOr("COLLABORATION_SERVICE_HTTP_ADDR", ":8002")
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// This socket is NOT proxied — a browser reaches it directly — so it
	// verifies tokens itself rather than trusting anything the gateway did
	// (ADR-013 §1). Same JWKS, same local verification, no per-connect RPC.
	verifier := authverify.New(envconfig.EnvOr(
		"AUTH_SERVICE_JWKS_URL", "http://localhost:8006/.well-known/jwks.json"))

	mux.HandleFunc("/collab/pages/{id}", wsapi.NewHandler(manager, wsAcceptOptions(), verifier, ws, ws).ServeHTTP)
	// These read endpoints return a page's OP LOG — its content, its edit
	// history, and who made each edit. They are not proxied either, so they
	// verify the same token the socket does (ADR-013 §1). Leaving them open
	// while the socket was closed would just move the hole.
	mux.HandleFunc("/collab/pages/{id}/trace", wsapi.RequireToken(verifier, wsapi.NewTraceHandler(repo, serverActor)))
	mux.HandleFunc("/collab/pages/{id}/blocks/{blockId}/palimpsest", wsapi.RequireToken(verifier, wsapi.NewPalimpsestHandler(repo, serverActor)))
	mux.HandleFunc("/collab/pages/{id}/diff", wsapi.RequireToken(verifier, wsapi.NewDiffHandler(repo, serverActor)))

	// Deliberately PUBLIC. § 02 HOME is the one route reachable without a
	// session, and its live counters come from here — instance-level totals
	// (sessions open, ops flushed, queue depth), never a page's content.
	// "How busy is this server" is not a secret; "what does this page say"
	// is, which is the line the three routes above sit on the other side of.
	mux.HandleFunc("/collab/stats", wsapi.NewStatsHandler(pool, manager))

	mux.HandleFunc("/collab/audit", wsapi.RequireToken(verifier, wsapi.NewAuditHandler(pool)))
	// § 23b PROFILE — a person as their op log. Behind the token like every
	// other read here: who edited what and when is a page's content by
	// another name.
	mux.HandleFunc("/collab/people/{id}/profile", wsapi.RequireToken(verifier, wsapi.NewProfileHandler(pool)))

	httpServer := &http.Server{Addr: httpAddr, Handler: allowCORS(mux)}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("collaboration-service listening", "addr", httpAddr)
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return httpServer.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

// allowCORS covers this mux's plain HTTP routes (the WebSocket upgrade
// itself is governed separately, by wsAcceptOptions' own cross-origin
// check below — coder/websocket never consults this header). A real
// gap this one: GET /collab/pages/{id}/trace|diff and .../palimpsest
// were reachable by curl from day one (curl doesn't enforce CORS) but
// silently failed from the actual browser History/Trace/Diff screens
// call them from — a cross-origin fetch with no
// Access-Control-Allow-Origin response header never reaches the
// caller's .then/.catch at all, it fails at the browser's own network
// layer. "*" is the same safe default api-gateway's own CORS setup
// already uses and for the same reason: nothing on this transport uses
// cookies or any other ambient credential a wildcard origin could leak
// (actor identity is the unauthenticated header/query stand-in pages.md/
// auth.md document).
func allowCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", http.MethodGet)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// wsAcceptOptions controls coder/websocket's cross-origin check.
// web/ is served from a different origin than this service (a different
// port, in local dev) — every real browser connection is cross-origin by
// this library's own definition, and its default (nil options) rejects
// that outright. COLLAB_ALLOWED_ORIGINS (comma-separated host patterns,
// e.g. "localhost:5173") sets a real allowlist; left unset, this defaults
// to skipping the check entirely — safe at this repo's scope because
// nothing in docs/api/collaboration.md's protocol uses cookies or any
// other ambient credential an arbitrary origin could leak (actor identity
// is the same unauthenticated header/query stand-in pages.md/auth.md
// document), the same reasoning api-gateway's own CORS default uses.
func wsAcceptOptions() *websocket.AcceptOptions {
	raw := os.Getenv("COLLAB_ALLOWED_ORIGINS")
	if raw == "" {
		return &websocket.AcceptOptions{InsecureSkipVerify: true}
	}
	patterns := strings.Split(raw, ",")
	for i, p := range patterns {
		patterns[i] = strings.TrimSpace(p)
	}
	return &websocket.AcceptOptions{OriginPatterns: patterns}
}
