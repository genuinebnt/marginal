//go:build integration

package authservice_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"marginal/auth-service/internal/authservice"
	"marginal/auth-service/internal/blocklist"
	"marginal/auth-service/internal/domain"
	"marginal/auth-service/internal/keys"
	"marginal/auth-service/internal/lockout"
	"marginal/auth-service/internal/migrate"
	"marginal/auth-service/internal/passwordhash"
	"marginal/auth-service/internal/sessions"
	"marginal/auth-service/internal/users"
)

// Real Postgres 18 + Redis via testcontainers-go — never a mock
// (.agents/agents.md). Run with: go test -tags=integration ./internal/authservice/...

// testParams keeps the suite fast — passwordhash's own unit tests already
// exercise DefaultParams' real cost and the timing property.
var testParams = passwordhash.Params{Memory: 8 * 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16}

// harness exposes the raw Redis client and key store alongside the
// Service under test, so a test can verify a side effect (e.g. "was this
// jti blocklisted") that Service's own API doesn't surface directly.
type harness struct {
	*authservice.Service
	rdb  *redis.Client
	keys keys.Store
	pool *pgxpool.Pool
}

func newTestService(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("auth_service_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(context.Background()) })
	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, migrate.Up(sqlDB))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	redisContainer, err := tcredis.Run(ctx, "redis:8-alpine")
	require.NoError(t, err)
	t.Cleanup(func() { _ = redisContainer.Terminate(context.Background()) })
	redisURI, err := redisContainer.ConnectionString(ctx)
	require.NoError(t, err)
	opts, err := redis.ParseURL(redisURI)
	require.NoError(t, err)
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })

	keyStore, err := keys.NewInMemoryStore()
	require.NoError(t, err)
	dummy, err := passwordhash.NewDummy(testParams)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	svc := authservice.New(pool, keyStore, testParams, dummy, blocklist.New(rdb), lockout.New(rdb), logger)
	return &harness{Service: svc, rdb: rdb, keys: keyStore, pool: pool}
}

// testWriter routes the service's slog output through t.Log so a failing
// test's WARN/INFO lines show up attributed to it.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}

func mustEmail(t *testing.T, raw string) domain.Email {
	t.Helper()
	e, err := domain.NewEmail(raw)
	require.NoError(t, err)
	return e
}

func mustPassword(t *testing.T, raw string) domain.Password {
	t.Helper()
	p, err := domain.NewPassword(raw)
	require.NoError(t, err)
	return p
}

func mustDisplayName(t *testing.T, raw string) domain.DisplayName {
	t.Helper()
	d, err := domain.NewDisplayName(raw)
	require.NoError(t, err)
	return d
}

// TestRegisterAllowsMultipleDistinctUsers pins the current model: ordinary,
// repeatable signup (docs/porting/PROGRESS.md), reversed from the earlier
// one-time-bootstrap-administrator design — real multi-user collaboration
// needs more than one person to be able to get an account without an
// operator running a side-channel tool per person.
func TestRegisterAllowsMultipleDistinctUsers(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	first, pair, err := svc.Register(ctx, mustEmail(t, "first@example.com"), mustPassword(t, "correct horse battery staple"), mustDisplayName(t, "First"))
	require.NoError(t, err)
	require.NotEmpty(t, pair.AccessToken)
	require.NotEmpty(t, pair.RefreshToken)
	require.Equal(t, "first@example.com", first.Email.String())

	second, _, err := svc.Register(ctx, mustEmail(t, "second@example.com"), mustPassword(t, "another long passphrase"), mustDisplayName(t, "Second"))
	require.NoError(t, err, "a second, distinct person must be able to register their own account")
	require.Equal(t, "second@example.com", second.Email.String())
	require.NotEqual(t, first.ID, second.ID)
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, _, err := svc.Register(ctx, mustEmail(t, "same@example.com"), mustPassword(t, "correct horse battery staple"), mustDisplayName(t, "First"))
	require.NoError(t, err)

	_, _, err = svc.Register(ctx, mustEmail(t, "same@example.com"), mustPassword(t, "a different passphrase"), mustDisplayName(t, "Impostor"))
	require.ErrorIs(t, err, users.ErrEmailTaken)
}

// TestRegisterWritesUserRegisteredOutboxEvent confirms outbox.WriteUserRegistered
// actually lands in the same transaction as the user insert — not just
// that it compiles, but that a real row with the right shape exists
// afterward, unpublished, ready for the poller.
func TestRegisterWritesUserRegisteredOutboxEvent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	user, _, err := svc.Register(ctx, mustEmail(t, "admin@example.com"), mustPassword(t, "correct horse battery staple"), mustDisplayName(t, "Admin"))
	require.NoError(t, err)

	var eventType string
	var aggregateID string
	var publishedAt *string
	var payload []byte
	err = svc.pool.QueryRow(ctx,
		`SELECT event_type, aggregate_id::text, published_at::text, payload FROM auth.outbox WHERE aggregate_id = $1`,
		user.ID.String(),
	).Scan(&eventType, &aggregateID, &publishedAt, &payload)
	require.NoError(t, err)

	require.Equal(t, "auth.user_registered", eventType)
	require.Equal(t, user.ID.String(), aggregateID)
	require.Nil(t, publishedAt, "must start unpublished — the poller's job, not Register's")

	var decoded struct {
		UserID      string `json:"user_id"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}
	require.NoError(t, json.Unmarshal(payload, &decoded))
	assert.Equal(t, user.ID.String(), decoded.UserID)
	assert.Equal(t, "admin@example.com", decoded.Email)
	assert.Equal(t, "Admin", decoded.DisplayName)
}

// TestConcurrentRegisterWithDistinctEmailsAllSucceed is the new model's
// version of the old bootstrap race test: many people signing up at once
// must all get their own account — nothing should serialize them onto a
// single winner anymore.
func TestConcurrentRegisterWithDistinctEmailsAllSucceed(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	const attempts = 10
	var succeeded atomic.Int32
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(i int) {
			defer wg.Done()
			_, _, err := svc.Register(ctx,
				mustEmail(t, fmt.Sprintf("claimant%d@example.com", i)),
				mustPassword(t, "correct horse battery staple"),
				mustDisplayName(t, "Claimant"))
			if err == nil {
				succeeded.Add(1)
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()

	require.EqualValues(t, attempts, succeeded.Load(), "every distinct-email registration must succeed")
}

// TestConcurrentRegisterWithSameEmailAllowsOnlyOne proves the UNIQUE(email)
// constraint itself still serializes the one case that must never double
// up — two people (or one person double-submitting) racing to claim the
// same email.
func TestConcurrentRegisterWithSameEmailAllowsOnlyOne(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	const attempts = 10
	var succeeded atomic.Int32
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			_, _, err := svc.Register(ctx,
				mustEmail(t, "contested@example.com"),
				mustPassword(t, "correct horse battery staple"),
				mustDisplayName(t, "Claimant"))
			if err == nil {
				succeeded.Add(1)
			} else if !errors.Is(err, users.ErrEmailTaken) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	require.EqualValues(t, 1, succeeded.Load(), "exactly one registration for the same email must win")
}

func TestAuthenticateWrongPasswordAndUnknownEmailBothFail(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	email := mustEmail(t, "alice@example.com")
	_, _, err := svc.Register(ctx, email, mustPassword(t, "correct horse battery staple"), mustDisplayName(t, "Alice"))
	require.NoError(t, err)

	_, _, err = svc.Authenticate(ctx, email, mustPassword(t, "wrong password entirely"))
	require.ErrorIs(t, err, authservice.ErrInvalidCredentials)

	_, _, err = svc.Authenticate(ctx, mustEmail(t, "nobody@example.com"), mustPassword(t, "irrelevant password"))
	require.ErrorIs(t, err, authservice.ErrInvalidCredentials)

	_, pair, err := svc.Authenticate(ctx, email, mustPassword(t, "correct horse battery staple"))
	require.NoError(t, err)
	require.NotEmpty(t, pair.AccessToken)
}

func TestRepeatedFailuresLockTheAccountEvenWithTheRightPasswordAfter(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	email := mustEmail(t, "bob@example.com")
	correct := mustPassword(t, "correct horse battery staple")
	_, _, err := svc.Register(ctx, email, correct, mustDisplayName(t, "Bob"))
	require.NoError(t, err)

	for i := 0; i < lockout.FailuresBeforeLock; i++ {
		_, _, err := svc.Authenticate(ctx, email, mustPassword(t, "wrong password entirely"))
		require.ErrorIs(t, err, authservice.ErrInvalidCredentials)
	}

	// The account is now locked — even the correct password is refused,
	// with the identical error, so lockout state isn't an oracle.
	_, _, err = svc.Authenticate(ctx, email, correct)
	require.ErrorIs(t, err, authservice.ErrInvalidCredentials)
}

func TestRefreshRotationHappyPath(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_, first, err := svc.Register(ctx, mustEmail(t, "carol@example.com"), mustPassword(t, "correct horse battery staple"), mustDisplayName(t, "Carol"))
	require.NoError(t, err)

	second, err := svc.Refresh(ctx, first.RefreshToken)
	require.NoError(t, err)
	require.NotEqual(t, first.RefreshToken, second.RefreshToken)
	require.NotEmpty(t, second.AccessToken)

	// The old refresh token is now consumed — presenting it again must be
	// rejected (and the family is revoked, verified in the reuse test).
	_, err = svc.Refresh(ctx, first.RefreshToken)
	require.ErrorIs(t, err, authservice.ErrSessionExpired)
}

// TestReuseOfARotatedTokenRevokesTheWholeFamily is the test named in
// docs/architecture/lld/auth-service.md §10: rotate three times, replay
// token #1, then assert all four are revoked and a fresh refresh with #4
// fails too.
func TestReuseOfARotatedTokenRevokesTheWholeFamily(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_, gen1, err := svc.Register(ctx, mustEmail(t, "dave@example.com"), mustPassword(t, "correct horse battery staple"), mustDisplayName(t, "Dave"))
	require.NoError(t, err)

	gen2, err := svc.Refresh(ctx, gen1.RefreshToken)
	require.NoError(t, err)
	gen3, err := svc.Refresh(ctx, gen2.RefreshToken)
	require.NoError(t, err)
	gen4, err := svc.Refresh(ctx, gen3.RefreshToken)
	require.NoError(t, err)

	// Replay the very first (already-rotated) token.
	_, err = svc.Refresh(ctx, gen1.RefreshToken)
	require.ErrorIs(t, err, authservice.ErrSessionExpired)

	// The whole chain, including the CURRENT tip (gen4), must now be dead.
	for i, tok := range []string{gen2.RefreshToken, gen3.RefreshToken, gen4.RefreshToken} {
		_, err := svc.Refresh(ctx, tok)
		require.ErrorIsf(t, err, authservice.ErrSessionExpired, "generation %d must be revoked too", i+2)
	}
}

func TestRevokeEndsOneSessionWithoutAffectingOthers(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	_, sessionA, err := svc.Register(ctx, mustEmail(t, "erin@example.com"), mustPassword(t, "correct horse battery staple"), mustDisplayName(t, "Erin"))
	require.NoError(t, err)

	// A second, independent login/session.
	_, sessionB, err := svc.Authenticate(ctx, mustEmail(t, "erin@example.com"), mustPassword(t, "correct horse battery staple"))
	require.NoError(t, err)

	require.NoError(t, svc.Revoke(ctx, sessionA.RefreshToken, nil))

	_, err = svc.Refresh(ctx, sessionA.RefreshToken)
	require.ErrorIs(t, err, authservice.ErrSessionExpired)

	// sessionB is untouched.
	_, err = svc.Refresh(ctx, sessionB.RefreshToken)
	require.NoError(t, err)
}

func TestRevokeBlocklistsTheSuppliedAccessToken(t *testing.T) {
	h := newTestService(t)
	ctx := context.Background()
	_, pair, err := h.Register(ctx, mustEmail(t, "frank@example.com"), mustPassword(t, "correct horse battery staple"), mustDisplayName(t, "Frank"))
	require.NoError(t, err)

	claims, err := sessions.Verify(h.keys, pair.AccessToken)
	require.NoError(t, err)

	bl := blocklist.New(h.rdb)
	blockedBefore, err := bl.IsBlocked(ctx, claims.Jti)
	require.NoError(t, err)
	require.False(t, blockedBefore, "must not be blocklisted before Revoke")

	accessToken := pair.AccessToken
	require.NoError(t, h.Revoke(ctx, pair.RefreshToken, &accessToken))

	blockedAfter, err := bl.IsBlocked(ctx, claims.Jti)
	require.NoError(t, err)
	require.True(t, blockedAfter, "Revoke with an access token must blocklist its jti")
}

func TestRevokeAllEndsEverySessionForTheCaller(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	user, sessionA, err := svc.Register(ctx, mustEmail(t, "grace@example.com"), mustPassword(t, "correct horse battery staple"), mustDisplayName(t, "Grace"))
	require.NoError(t, err)
	_, sessionB, err := svc.Authenticate(ctx, mustEmail(t, "grace@example.com"), mustPassword(t, "correct horse battery staple"))
	require.NoError(t, err)

	require.NoError(t, svc.RevokeAll(ctx, user.ID))

	_, err = svc.Refresh(ctx, sessionA.RefreshToken)
	require.ErrorIs(t, err, authservice.ErrSessionExpired)
	_, err = svc.Refresh(ctx, sessionB.RefreshToken)
	require.ErrorIs(t, err, authservice.ErrSessionExpired)
}
