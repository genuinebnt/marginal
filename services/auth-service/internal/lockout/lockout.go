// Package lockout is per-account login-failure tracking (Redis) — LLD
// §12: "Rate limiting belongs at the gateway, but the counter belongs
// here." Per-IP limiting defeats itself against an attacker distributing
// attempts across many IPs; per-account lockout doesn't. See
// docs/api/auth.md § Product-facing behavior for the thresholds.
package lockout

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"marginal/auth-service/internal/domain"
)

const (
	// FailuresBeforeLock is docs/api/auth.md's decided threshold — OWASP
	// Authentication Cheat Sheet's exponential-backoff guidance.
	FailuresBeforeLock = 5
	attemptWindow      = time.Hour // failures older than this don't accumulate
)

type Store struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Store { return &Store{rdb: rdb} }

func attemptsKey(id domain.UserID) string { return "lockout:attempts:" + id.String() }
func lockedKey(id domain.UserID) string   { return "lockout:locked:" + id.String() }

// backoff maps the failure count to how long the account stays locked —
// capped at 30 minutes, matching docs/api/auth.md's decided policy
// (1 / 5 / 15 / 30 minutes as attempts climb past the threshold).
func backoff(attempts int64) time.Duration {
	switch {
	case attempts < FailuresBeforeLock+5:
		return time.Minute
	case attempts < FailuresBeforeLock+10:
		return 5 * time.Minute
	case attempts < FailuresBeforeLock+15:
		return 15 * time.Minute
	default:
		return 30 * time.Minute
	}
}

// RecordFailure increments id's failure counter and locks the account
// once FailuresBeforeLock is reached, applying the backoff for the
// current attempt count.
func (s *Store) RecordFailure(ctx context.Context, id domain.UserID) error {
	n, err := s.rdb.Incr(ctx, attemptsKey(id)).Result()
	if err != nil {
		return fmt.Errorf("lockout: recording failure: %w", err)
	}
	if n == 1 {
		if err := s.rdb.Expire(ctx, attemptsKey(id), attemptWindow).Err(); err != nil {
			return fmt.Errorf("lockout: setting attempt window: %w", err)
		}
	}
	if n < FailuresBeforeLock {
		return nil
	}
	if err := s.rdb.Set(ctx, lockedKey(id), "1", backoff(n)).Err(); err != nil {
		return fmt.Errorf("lockout: locking account: %w", err)
	}
	return nil
}

func (s *Store) IsLocked(ctx context.Context, id domain.UserID) (bool, error) {
	n, err := s.rdb.Exists(ctx, lockedKey(id)).Result()
	if err != nil {
		return false, fmt.Errorf("lockout: checking lock: %w", err)
	}
	return n > 0, nil
}

// Clear resets id's failure count and lock — called on a successful
// authentication.
func (s *Store) Clear(ctx context.Context, id domain.UserID) error {
	if err := s.rdb.Del(ctx, attemptsKey(id), lockedKey(id)).Err(); err != nil {
		return fmt.Errorf("lockout: clearing: %w", err)
	}
	return nil
}
