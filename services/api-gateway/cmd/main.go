// Command api-gateway is a thin REST↔gRPC shim for document-service's
// PageService and auth-service's AuthService — not the full-featured
// api-gateway from the original 11-service design (out of this repo's
// scope per ADR-011), just enough of one to make those two gRPC-only
// services reachable from a browser. See docs/api/pages.md §2 and
// docs/api/auth.md §2 for the REST contract this implements.
//
// collaboration-service is not proxied here — its WebSocket endpoint is
// a persistent connection, not a request/response resource, so it's
// reached directly (see docs/api/collaboration.md).
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"marginal/authverify"
	"marginal/envconfig"

	authv1 "marginal/auth-service/genproto/authv1"
	diagnosticsv1 "marginal/diagnostics-service/genproto/diagnosticsv1"
	documentv1 "marginal/document-service/genproto/documentv1"

	"marginal/api-gateway/internal/adminrest"
	"marginal/api-gateway/internal/authmw"
	"marginal/api-gateway/internal/authrest"
	"marginal/api-gateway/internal/diagnosticsrest"
	"marginal/api-gateway/internal/discoverrest"
	"marginal/api-gateway/internal/graphrest"
	"marginal/api-gateway/internal/pagesrest"
	"marginal/api-gateway/internal/searchrest"
)

// requestTimeout bounds every request this gateway handles, which in turn
// bounds every outbound gRPC call made through actorctx.FromRequest — it
// derives its context from r.Context(), and middleware.Timeout replaces
// that with a context.WithTimeout-wrapped one before any handler runs.
// Without this, a stuck document-service/auth-service call had no
// deadline of its own and could hold the handler goroutine indefinitely.
const requestTimeout = 15 * time.Second

// maxRequestBodyBytes bounds every request body this gateway reads before
// decoding it as JSON — generous for this API's actual payloads (a page
// title alone is capped at 500 bytes, documentcore's maxTitleBytes) but
// bounded, so an oversized or malicious body can't be read into memory
// in full before json.Decode ever gets to reject it.
const maxRequestBodyBytes = 1 << 20 // 1 MiB

func limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		next.ServeHTTP(w, r)
	})
}

func main() {
	if err := run(); err != nil {
		slog.Error("api-gateway failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	documentAddr := envconfig.EnvOr("DOCUMENT_SERVICE_GRPC_ADDR", "localhost:9001")
	authAddr := envconfig.EnvOr("AUTH_SERVICE_GRPC_ADDR", "localhost:9006")
	diagnosticsAddr := envconfig.EnvOr("DIAGNOSTICS_SERVICE_GRPC_ADDR", "localhost:9008")

	// insecure.NewCredentials(): east-west traffic within this repo's
	// deployment topology (ADR-007) — TLS termination, if any, happens at
	// the network edge, not between the gateway and its backends.
	documentConn, err := grpc.NewClient(documentAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = documentConn.Close() }()

	authConn, err := grpc.NewClient(authAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = authConn.Close() }()

	diagnosticsConn, err := grpc.NewClient(diagnosticsAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = diagnosticsConn.Close() }()

	pages := pagesrest.NewHandler(documentv1.NewPageServiceClient(documentConn))
	graphHandler := graphrest.NewHandler(documentv1.NewGraphServiceClient(documentConn))
	searchHandler := searchrest.NewHandler(documentv1.NewSearchServiceClient(documentConn))
	discoverHandler := discoverrest.NewHandler(documentv1.NewDiscoverServiceClient(documentConn))
	diagnosticsHandler := diagnosticsrest.NewHandler(diagnosticsv1.NewDiagnosticsServiceClient(diagnosticsConn))
	auth := authrest.NewHandler(authv1.NewAuthServiceClient(authConn))

	// § 18 ADMIN's SERVICES panel. HTTP health endpoints, not the
	// gRPC ones: the question is "is the process answering", and
	// every service in this repo exposes exactly one plain probe
	// for that. Addresses come from the environment so the list
	// follows the deployment rather than a constant compiled in
	// here.
	admin := adminrest.NewHandler(map[string]string{
		"document-service":      envconfig.EnvOr("DOCUMENT_SERVICE_HEALTH_URL", "http://localhost:8001/health"),
		"auth-service":          envconfig.EnvOr("AUTH_SERVICE_HEALTH_URL", "http://localhost:8006/health"),
		"collaboration-service": envconfig.EnvOr("COLLABORATION_SERVICE_HEALTH_URL", "http://localhost:8002/health"),
		"notification-service":  envconfig.EnvOr("NOTIFICATION_SERVICE_HEALTH_URL", "http://localhost:8007/health"),
		"diagnostics-service":   envconfig.EnvOr("DIAGNOSTICS_SERVICE_HEALTH_URL", "http://localhost:8008/health"),
	})

	// Tokens are verified HERE, locally, against auth-service's published
	// JWKS — never by asking auth-service about each one, which would put a
	// second hop in front of every request and make every screen depend on
	// auth-service being up (ADR-013 §1).
	verifier := authverify.New(envconfig.EnvOr(
		"AUTH_SERVICE_JWKS_URL", "http://localhost:8006/.well-known/jwks.json"))

	r := chi.NewRouter()
	r.Use(middleware.Timeout(requestTimeout))
	r.Use(limitRequestBody)
	r.Use(cors.Handler(cors.Options{
		// A browser calling this gateway from web/ (a different origin —
		// different port, same host in local dev) is a cross-origin
		// request; without this, every fetch from the SPA fails at the
		// browser's own CORS check before this service's handlers ever
		// see it (docs/porting/PROGRESS.md's frontend entry — this was
		// found by actually trying it in a browser, not by any test here,
		// since none of this repo's own tests exercise cross-origin
		// behavior). Origins configurable via CORS_ALLOWED_ORIGINS
		// (comma-separated); "*" is the safe default at this repo's
		// scope: no cookie/credentialed request exists anywhere in this
		// codebase (auth is a bearer-less header/query stand-in — see
		// pages.md/auth.md's "Actor identity" sections), so there is no
		// credential a wildcard origin could leak.
		//
		// ADR-013 changed the second half of that reasoning and it is worth
		// being precise rather than leaving a stale justification in place.
		// There IS a credential now — the bearer token. A wildcard origin is
		// still not a leak, but for a different reason than before: a bearer
		// token is attached by the page's own code, never automatically by
		// the browser the way a cookie is, and AllowCredentials stays false
		// so no ambient credential can ride along. If this ever moves to
		// cookie auth, "*" stops being safe the same day.
		AllowedOrigins: allowedOrigins(),
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		// X-Actor-Id and X-Actor-Kind are gone: nothing reads them any more,
		// and advertising a header the server ignores invites a client to
		// keep sending it and believe it means something.
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	// After CORS (so a preflight is answered rather than rejected) and
	// before every route: this is the line that makes actorctx's identity a
	// verified one instead of a claimed one.
	r.Use(authmw.Middleware(verifier))

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	pages.Mount(r)
	graphHandler.Mount(r)
	searchHandler.Mount(r)
	discoverHandler.Mount(r)
	diagnosticsHandler.Mount(r)
	auth.Mount(r)
	admin.Mount(r)

	addr := envconfig.EnvOr("API_GATEWAY_HTTP_ADDR", ":8000")
	slog.Info("api-gateway listening", "addr", addr, "document_service", documentAddr, "auth_service", authAddr, "diagnostics_service", diagnosticsAddr)
	return http.ListenAndServe(addr, r)
}

func allowedOrigins() []string {
	raw := envconfig.EnvOr("CORS_ALLOWED_ORIGINS", "*")
	parts := strings.Split(raw, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}
