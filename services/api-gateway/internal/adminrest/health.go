// Package adminrest is § 18 ADMIN's data: which services are up,
// and how fast they answer.
//
// It probes rather than reports. There is no service registry in
// this repo and no orchestrator to ask, so "healthy" means one
// thing only — this gateway asked, and got an answer in time.
// That is a weaker claim than a real health check makes (nothing
// here knows whether a service's database is reachable, only
// whether its process is answering), and the screen says so
// rather than borrowing the credibility of a word like
// "healthy" without earning it.
package adminrest

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"marginal/api-gateway/internal/apierror"
)

// probeTimeout bounds one service probe. Short on purpose: this
// answers a screen, and a screen that waits ten seconds to tell
// you something is down has already told you.
const probeTimeout = 2 * time.Second

// Service is one row of § 18's SERVICES panel.
type Service struct {
	Name string `json:"name"`
	// URL is shown so the row is actionable — a name alone
	// cannot be curl'd.
	URL string `json:"url"`
	// Status is "up", "down", or "timeout". Three values rather
	// than a boolean because they need three different next
	// actions: nothing, look at the logs, look at the network.
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	// Detail carries the failure when there is one — the error
	// string or the status code. Empty when up.
	Detail string `json:"detail,omitempty"`
	// Role is what the service is for, in three words. § 18's
	// mockup put "4 instances" here, which this deployment
	// cannot honestly say: it runs one of each.
	Role string `json:"role"`
}

type healthResponse struct {
	Services []Service `json:"services"`
	Up       int       `json:"up"`
	Total    int       `json:"total"`
	// CheckedAt is when the probe ran, so a stale panel is
	// visibly stale rather than silently old.
	CheckedAt time.Time `json:"checked_at"`
}

// Handler probes a fixed list of health endpoints.
type Handler struct {
	targets []target
	client  *http.Client
}

type target struct{ name, url, role string }

// NewHandler takes the health URLs from configuration, so the
// probe list follows the deployment rather than a constant
// compiled into the gateway.
func NewHandler(urls map[string]string) *Handler {
	roles := map[string]string{
		"document-service":      "pages, blocks, graph, search",
		"auth-service":          "identity and tokens",
		"collaboration-service": "live sessions and the op log",
		"notification-service":  "the inbox",
		"diagnostics-service":   "prose checks, no database",
		"api-gateway":           "this process",
	}
	h := &Handler{client: &http.Client{Timeout: probeTimeout}}
	for name, url := range urls {
		h.targets = append(h.targets, target{name: name, url: url, role: roles[name]})
	}
	sort.Slice(h.targets, func(i, j int) bool { return h.targets[i].name < h.targets[j].name })
	return h
}

func (h *Handler) Mount(r chi.Router) {
	r.Get("/admin/health", h.health)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	// Probed concurrently: six sequential 2-second timeouts is
	// twelve seconds of a screen doing nothing, and the probes
	// are independent.
	out := make([]Service, len(h.targets))
	var wg sync.WaitGroup
	for i, t := range h.targets {
		wg.Add(1)
		go func(i int, t target) {
			defer wg.Done()
			out[i] = h.probe(r.Context(), t)
		}(i, t)
	}
	wg.Wait()

	resp := healthResponse{Services: out, Total: len(out), CheckedAt: time.Now().UTC()}
	for _, s := range out {
		if s.Status == "up" {
			resp.Up++
		}
	}
	apierror.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) probe(ctx context.Context, t target) Service {
	s := Service{Name: t.name, URL: t.url, Role: t.role}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
	if err != nil {
		s.Status, s.Detail = "down", err.Error()
		return s
	}

	start := time.Now()
	res, err := h.client.Do(req)
	s.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		// A deadline and a refused connection are different
		// problems with different fixes, so they get different
		// words.
		if ctx.Err() != nil {
			s.Status, s.Detail = "timeout", "no answer within 2s"
		} else {
			s.Status, s.Detail = "down", err.Error()
		}
		return s
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		s.Status, s.Detail = "down", res.Status
		return s
	}
	s.Status = "up"
	return s
}
