// Package authservice orchestrates users + sessions + keys + Redis into
// the use cases the gRPC layer calls — Register, Authenticate, GetUser,
// Refresh, Revoke, RevokeAll. This is where transactions spanning more
// than one table are opened and committed (registration's user-insert +
// first-refresh-token-insert; rotation's revoke-old + insert-new), and
// where the credential-timing and lockout invariants live. api.go stays a
// thin proto <-> domain translation layer over this.
package authservice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"marginal/auth-service/internal/authrepo/gen"
	"marginal/auth-service/internal/blocklist"
	"marginal/auth-service/internal/domain"
	"marginal/auth-service/internal/keys"
	"marginal/auth-service/internal/lockout"
	"marginal/auth-service/internal/outbox"
	"marginal/auth-service/internal/passwordhash"
	"marginal/auth-service/internal/sessions"
	"marginal/auth-service/internal/users"
)

var (
	// ErrInvalidCredentials covers unknown email, wrong password, and a
	// locked account alike — deliberately one error, one message
	// (docs/api/auth.md § Status codes: the two UNAUTHENTICATED cases
	// "must be byte-identical", and a locked account returns the same
	// message so lockout state isn't its own oracle).
	ErrInvalidCredentials = errors.New("authservice: invalid credentials")
	// ErrSessionExpired covers an unrecognized, expired, or reused
	// refresh token — same reasoning as ErrInvalidCredentials: the
	// attacker who replayed a consumed token learns nothing beyond what a
	// legitimately expired session would also show.
	ErrSessionExpired = errors.New("authservice: session expired")
)

type Service struct {
	pool         *pgxpool.Pool
	keys         keys.Store
	argon2Params passwordhash.Params
	dummy        *passwordhash.Dummy
	blocklist    *blocklist.Store
	lockout      *lockout.Store
	log          *slog.Logger
}

func New(
	pool *pgxpool.Pool,
	keyStore keys.Store,
	argon2Params passwordhash.Params,
	dummy *passwordhash.Dummy,
	blocklistStore *blocklist.Store,
	lockoutStore *lockout.Store,
	logger *slog.Logger,
) *Service {
	return &Service{
		pool:         pool,
		keys:         keyStore,
		argon2Params: argon2Params,
		dummy:        dummy,
		blocklist:    blocklistStore,
		lockout:      lockoutStore,
		log:          logger,
	}
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64 // seconds
}

// Register is ordinary, repeatable signup (ADR-011 addendum: shared
// single workspace, real multi-user, not the earlier one-time-bootstrap-
// administrator model — every account this creates has identical
// standing, there is no "admin"; see docs/porting/PROGRESS.md for why
// that changed). Uniqueness is enforced by auth.users' own UNIQUE(email)
// constraint (users.Insert maps a collision to ErrEmailTaken) — no
// upfront existence check first, which would be a second, redundant
// round trip and a TOCTOU window of its own.
func (s *Service) Register(ctx context.Context, email domain.Email, password domain.Password, displayName domain.DisplayName) (users.User, TokenPair, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return users.User{}, TokenPair{}, fmt.Errorf("authservice: register: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	q := authrepo.New(tx)

	hash, err := passwordhash.Hash(password, s.argon2Params)
	if err != nil {
		return users.User{}, TokenPair{}, fmt.Errorf("authservice: register: hashing password: %w", err)
	}

	id := domain.NewUserID()
	user, err := users.Insert(ctx, q, users.NewUser{
		ID:           id,
		Email:        email,
		PasswordHash: hash,
		DisplayName:  displayName,
		CursorColor:  domain.AssignCursorColor(id),
	})
	if err != nil {
		return users.User{}, TokenPair{}, err
	}

	pair, err := issueTokenPair(ctx, q, s.keys, id)
	if err != nil {
		return users.User{}, TokenPair{}, err
	}

	if err := outbox.WriteUserRegistered(ctx, q, uuid.UUID(id), email.String(), displayName.String()); err != nil {
		return users.User{}, TokenPair{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return users.User{}, TokenPair{}, fmt.Errorf("authservice: register: commit: %w", err)
	}
	committed = true
	return user, pair, nil
}

// Authenticate always runs a real Argon2id verify against the stored
// hash — even for an unknown email (against a precomputed dummy hash) and
// even for a locked account — so the time to reject any of the three
// cases is indistinguishable (docs/architecture/lld/auth-service.md
// §9/§12). Only after that constant-cost check does a locked account or a
// wrong password get rejected, both with the identical
// ErrInvalidCredentials.
func (s *Service) Authenticate(ctx context.Context, email domain.Email, password domain.Password) (users.User, TokenPair, error) {
	q := authrepo.New(s.pool)

	user, err := users.FindByEmail(ctx, q, email)
	if errors.Is(err, users.ErrNotFound) {
		s.dummy.Verify(password)
		s.log.Info("authenticate: unknown email")
		return users.User{}, TokenPair{}, ErrInvalidCredentials
	}
	if err != nil {
		return users.User{}, TokenPair{}, err
	}

	locked, err := s.lockout.IsLocked(ctx, user.ID)
	if err != nil {
		return users.User{}, TokenPair{}, err
	}

	ok, err := passwordhash.Verify(user.PasswordHash, password)
	if err != nil {
		return users.User{}, TokenPair{}, err
	}

	if !ok {
		if err := s.lockout.RecordFailure(ctx, user.ID); err != nil {
			s.log.Error("authenticate: recording lockout failure", "err", err)
		}
		s.log.Info("authenticate: wrong password", "user_id", user.ID.String())
		return users.User{}, TokenPair{}, ErrInvalidCredentials
	}
	if locked {
		s.log.Info("authenticate: account locked", "user_id", user.ID.String())
		return users.User{}, TokenPair{}, ErrInvalidCredentials
	}

	if err := s.lockout.Clear(ctx, user.ID); err != nil {
		s.log.Error("authenticate: clearing lockout state", "err", err)
	}

	pair, err := issueTokenPair(ctx, q, s.keys, user.ID)
	if err != nil {
		return users.User{}, TokenPair{}, err
	}
	return user, pair, nil
}

func (s *Service) GetUser(ctx context.Context, id domain.UserID) (users.User, error) {
	return users.FindByID(ctx, authrepo.New(s.pool), id)
}

// Refresh runs the rotation state machine (internal/sessions.Rotate) in
// one transaction. Reuse detection still commits — the family revocation
// it performed is real and must persist — but returns ErrSessionExpired
// to the caller either way, logged at WARN specifically for that case
// (docs/api/auth.md § Status codes).
func (s *Service) Refresh(ctx context.Context, presented string) (TokenPair, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TokenPair{}, fmt.Errorf("authservice: refresh: begin: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	q := authrepo.New(tx)
	newPlain, result, rotErr := sessions.Rotate(ctx, q, presented)

	switch {
	case errors.Is(rotErr, sessions.ErrRefreshTokenReused):
		if err := tx.Commit(ctx); err != nil {
			return TokenPair{}, fmt.Errorf("authservice: refresh: commit reuse revocation: %w", err)
		}
		committed = true
		s.log.Warn("refresh: token reuse detected, family revoked",
			"user_id", result.UserID.String(), "family_revoked", result.FamilyRevoked)
		return TokenPair{}, ErrSessionExpired

	case errors.Is(rotErr, sessions.ErrRefreshTokenExpired):
		s.log.Info("refresh: expired token")
		return TokenPair{}, ErrSessionExpired

	case errors.Is(rotErr, sessions.ErrRefreshTokenInvalid):
		s.log.Info("refresh: unrecognized token")
		return TokenPair{}, ErrSessionExpired

	case rotErr != nil:
		return TokenPair{}, rotErr
	}

	access, _, err := sessions.Issue(s.keys, result.UserID)
	if err != nil {
		return TokenPair{}, fmt.Errorf("authservice: refresh: issuing access token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TokenPair{}, fmt.Errorf("authservice: refresh: commit: %w", err)
	}
	committed = true
	return TokenPair{
		AccessToken:  access,
		RefreshToken: newPlain,
		ExpiresIn:    int64(sessions.AccessTokenLifetime.Seconds()),
	}, nil
}

// Revoke ends one session: the refresh token's entire chain (whether
// presented token is the chain's root or a since-rotated descendant), and
// — if accessToken is supplied — blocklists its jti too. Blocklisting and
// chain revocation are separate mechanisms (LLD §12: "revoking on the
// blocklist is not revoking the refresh token... logout must do both") —
// this method is the one place that does both for a single logout.
func (s *Service) Revoke(ctx context.Context, refreshToken string, accessToken *string) error {
	q := authrepo.New(s.pool)
	hash := sessions.HashRefreshToken(refreshToken)

	row, err := sessions.FindByHash(ctx, q, hash)
	if errors.Is(err, sessions.ErrNotFound) {
		// Nothing to revoke, and — same reasoning as an unknown email —
		// no signal to a caller that might be probing for valid tokens.
	} else if err != nil {
		return err
	} else if _, err := sessions.RevokeChain(ctx, q, row.ID); err != nil {
		return err
	}

	if accessToken != nil {
		if claims, err := sessions.Verify(s.keys, *accessToken); err == nil {
			if ttl := time.Until(claims.Exp); ttl > 0 {
				if err := s.blocklist.Block(ctx, claims.Jti, ttl); err != nil {
					return fmt.Errorf("authservice: revoke: blocklisting access token: %w", err)
				}
			}
		}
		// An unverifiable access token (already expired, garbage) has
		// nothing left to blocklist — not an error for Revoke itself.
	}
	return nil
}

func (s *Service) RevokeAll(ctx context.Context, userID domain.UserID) error {
	_, err := sessions.RevokeAllForUser(ctx, authrepo.New(s.pool), userID)
	return err
}

func issueTokenPair(ctx context.Context, q *authrepo.Queries, store keys.Store, userID domain.UserID) (TokenPair, error) {
	access, _, err := sessions.Issue(store, userID)
	if err != nil {
		return TokenPair{}, fmt.Errorf("authservice: issuing access token: %w", err)
	}

	refreshPlain, refreshHash, err := sessions.NewRefreshToken()
	if err != nil {
		return TokenPair{}, fmt.Errorf("authservice: generating refresh token: %w", err)
	}
	if err := sessions.Insert(ctx, q, sessions.NewToken{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: refreshHash,
		ParentID:  nil,
		ExpiresAt: time.Now().Add(sessions.RefreshTokenLifetime),
	}); err != nil {
		return TokenPair{}, err
	}

	return TokenPair{
		AccessToken:  access,
		RefreshToken: refreshPlain,
		ExpiresIn:    int64(sessions.AccessTokenLifetime.Seconds()),
	}, nil
}
