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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

// Config is New's dependency set. A struct rather than seven positional
// parameters: every field here is required (none has a sensible
// zero-value default), which is exactly the case a Config struct suits —
// functional options earn their keep when most parameters are optional
// with sane defaults (this codebase's own convention for that case is
// collaboration-service/internal/outbox.Option/session.ManagerOption);
// here, named fields just make a call site self-documenting and immune
// to two same-typed arguments (Blocklist/Lockout, both *Store) getting
// silently transposed.
type Config struct {
	Pool         *pgxpool.Pool
	Keys         keys.Store
	Argon2Params passwordhash.Params
	Dummy        *passwordhash.Dummy
	Blocklist    *blocklist.Store
	Lockout      *lockout.Store
	Logger       *slog.Logger
}

func New(cfg Config) *Service {
	return &Service{
		pool:         cfg.Pool,
		keys:         cfg.Keys,
		argon2Params: cfg.Argon2Params,
		dummy:        cfg.Dummy,
		blocklist:    cfg.Blocklist,
		lockout:      cfg.Lockout,
		log:          cfg.Logger,
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
	// Rollback after a successful Commit is a safe no-op (pgx.ErrTxClosed,
	// discarded here) — no committed-bool guard needed, matching every
	// other transactional method in this codebase (e.g.
	// document-service/internal/pages.PostgresRepo.Reparent).
	defer func() { _ = tx.Rollback(ctx) }()

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
		return users.User{}, TokenPair{}, fmt.Errorf("authservice: register: creating user: %w", err)
	}

	pair, err := issueTokenPair(ctx, q, s.keys, id)
	if err != nil {
		return users.User{}, TokenPair{}, fmt.Errorf("authservice: register: %w", err)
	}

	if err := outbox.WriteUserRegistered(ctx, q, uuid.UUID(id), email.String(), displayName.String()); err != nil {
		return users.User{}, TokenPair{}, fmt.Errorf("authservice: register: writing outbox event: %w", err)
	}

	// A new account joins the default space as a VIEWER (ADR-013's own
	// consequence: "the showcase content lives in a space where new
	// accounts are viewer").
	//
	// Without this a fresh registration lands in no space at all and sees
	// an empty instance — technically correct enforcement and a terrible
	// front door. Viewer rather than editor is the actual tightening this
	// release buys: before spaces, anyone who registered could edit or
	// delete the seeded corpus, which is why ops/reseed.timer rebuilds it
	// nightly. That timer stays, but it stops being the only thing between
	// a visitor and the pages.
	//
	// In the SAME transaction as the user, with its own event: a member
	// row that is durable without its announcement leaves document-service
	// filtering reads against a projection that will never learn about it.
	if err := s.joinDefaultSpace(ctx, q, uuid.UUID(id)); err != nil {
		return users.User{}, TokenPair{}, fmt.Errorf("authservice: register: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return users.User{}, TokenPair{}, fmt.Errorf("authservice: register: commit: %w", err)
	}
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

// ListPeople is § 18 ADMIN's PEOPLE panel and its SESSIONS
// readout, in one call because the screen shows them together
// and two round trips would let them disagree on screen.
//
// No authorization check, and that is a gap rather than a
// design: RBAC is v3.1.0, so until it exists any authenticated
// actor can read this. The screen says so out loud rather than
// implying an admin surface that is actually open.
// AuthEvent is one § 18b audit row from this service's side.
type AuthEvent struct {
	ID     string
	Kind   string
	UserID string
	At     time.Time
}

// ListAuthEvents derives the auth half of § 18b's log.
//
// Nothing here is emitted; it is all read from state that
// already exists. Failed sign-in attempts are absent because
// nothing records them, and the screen states that rather than
// letting an empty column read as "there were none".
//
// The limit applies to sign-ins and sign-outs only —
// registrations are always all of them. An account being created
// is the most audit-worthy auth event there is, and a shared
// limit lets a few hundred routine sign-ins push every
// registration off the end, leaving the log showing only the
// noise. So the returned count can exceed `limit` by the number
// of people, which on a self-hosted instance is a number that
// fits.
func (s *Service) ListAuthEvents(ctx context.Context, limit int32) ([]AuthEvent, error) {
	if limit < 1 || limit > maxAuthEventLimit {
		limit = maxAuthEventLimit
	}
	rows, err := authrepo.New(s.pool).AuditAuthEvents(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("authservice: list auth events: %w", err)
	}
	out := make([]AuthEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, AuthEvent{
			ID:     uuid.UUID(r.ID.Bytes).String(),
			Kind:   r.Kind,
			UserID: uuid.UUID(r.UserID.Bytes).String(),
			At:     r.At.Time,
		})
	}
	return out, nil
}

const maxAuthEventLimit = 200

func (s *Service) ListPeople(ctx context.Context) ([]users.User, int64, error) {
	q := authrepo.New(s.pool)
	people, err := users.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	sessions, err := users.CountActiveSessions(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return people, sessions, nil
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
	defer func() { _ = tx.Rollback(ctx) }()

	q := authrepo.New(tx)
	newPlain, userID, rotErr := sessions.Rotate(ctx, q, presented)

	switch {
	case errors.Is(rotErr, sessions.ErrRefreshTokenReused):
		if err := tx.Commit(ctx); err != nil {
			return TokenPair{}, fmt.Errorf("authservice: refresh: commit reuse revocation: %w", err)
		}
		var reuseErr *sessions.ReuseDetectedError
		if errors.As(rotErr, &reuseErr) {
			s.log.Warn("refresh: token reuse detected, family revoked",
				"user_id", reuseErr.UserID.String(), "family_revoked", reuseErr.FamilyRevoked)
		}
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

	access, _, err := sessions.Issue(s.keys, userID)
	if err != nil {
		return TokenPair{}, fmt.Errorf("authservice: refresh: issuing access token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TokenPair{}, fmt.Errorf("authservice: refresh: commit: %w", err)
	}
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
		return TokenPair{}, fmt.Errorf("authservice: inserting refresh token: %w", err)
	}

	return TokenPair{
		AccessToken:  access,
		RefreshToken: refreshPlain,
		ExpiresIn:    int64(sessions.AccessTokenLifetime.Seconds()),
	}, nil
}

// DefaultSpaceID is the space every pre-v3.1.0 page was migrated into. The
// same constant appears in both services' migrations and in
// document-service's own code, which DATA_MODEL.md records as a deliberate
// coordination point: two services must agree which space existing pages
// belong to and cannot join across schemas to find out.
var DefaultSpaceID = uuid.MustParse("00000000-0000-7000-8000-00000000d0c5")

// joinDefaultSpace puts a newly registered user in the default space, and
// announces it.
//
// THE FIRST USER BOOTSTRAPS THE SPACE, AS ITS ADMIN.
//
// Migration 00002 creates the default space from the users that already
// exist — so on a FRESH database it creates nothing, and without this the
// first account would land in no space, see an empty instance, and have no
// way to fix that: granting requires an admin, and there would be none.
// The first registration is the only moment that bootstrap can happen.
//
// Everyone after that joins as a VIEWER (ADR-013's own consequence: "the
// showcase content lives in a space where new accounts are viewer"). That
// is the actual tightening this release buys — before spaces, anyone who
// registered could edit or delete the seeded corpus.
func (s *Service) joinDefaultSpace(ctx context.Context, q *authrepo.Queries, userID uuid.UUID) error {
	pgUser := pgtype.UUID{Bytes: userID, Valid: true}

	space, err := q.DefaultSpace(ctx)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Bootstrap: create it, with this user as its admin.
		created, err := q.CreateDefaultSpace(ctx, authrepo.CreateDefaultSpaceParams{
			ID:        pgtype.UUID{Bytes: DefaultSpaceID, Valid: true},
			Name:      "Workspace",
			CreatedBy: pgUser,
		})
		if err != nil {
			return fmt.Errorf("bootstrapping default space: %w", err)
		}
		space = created
	case err != nil:
		return fmt.Errorf("reading default space: %w", err)
	}

	role := "viewer"
	if uuid.UUID(space.CreatedBy.Bytes) == userID {
		role = "admin"
	}

	if _, err := q.UpsertMembership(ctx, authrepo.UpsertMembershipParams{
		UserID:    pgUser,
		SpaceID:   space.ID,
		Role:      role,
		GrantedBy: space.CreatedBy,
	}); err != nil {
		return fmt.Errorf("joining default space: %w", err)
	}
	return outbox.WriteRoleEvent(ctx, q, outbox.EventRoleGranted, outbox.RolePayload{
		UserID: userID, SpaceID: uuid.UUID(space.ID.Bytes), Role: role, GrantedAt: time.Now().UTC(),
	})
}
