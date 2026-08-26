// Package blocklist is the access-token revocation list — Redis, keyed
// jwt:blocklist:{jti}, matching DATA_MODEL.md §6 exactly. Access tokens
// are never stored server-side (they're short-lived JWTs verified
// locally), so this is the only way to invalidate one before its natural
// exp.
package blocklist

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"marginal/auth-service/internal/domain"
)

type Store struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Store { return &Store{rdb: rdb} }

func key(jti domain.Jti) string { return "jwt:blocklist:" + jti.String() }

// Block adds jti to the blocklist for ttl. ttl must be >= the token's
// remaining lifetime — a shorter TTL un-revokes it before it would have
// expired on its own (docs/architecture/lld/auth-service.md §9).
func (s *Store) Block(ctx context.Context, jti domain.Jti, ttl time.Duration) error {
	if err := s.rdb.Set(ctx, key(jti), "1", ttl).Err(); err != nil {
		return fmt.Errorf("blocklist: block: %w", err)
	}
	return nil
}

func (s *Store) IsBlocked(ctx context.Context, jti domain.Jti) (bool, error) {
	n, err := s.rdb.Exists(ctx, key(jti)).Result()
	if err != nil {
		return false, fmt.Errorf("blocklist: check: %w", err)
	}
	return n > 0, nil
}
