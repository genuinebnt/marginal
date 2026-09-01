// Package spaceproj maintains docs.space_members — this service's local
// answer to "which spaces may this reader see".
//
// It is a PROJECTION of auth.memberships and never a second source of
// truth. If the two disagree, auth is right and this is stale, which is
// exactly why the WRITE path does not consult it: collaboration-service's
// can_apply resolves a role from auth-service at join (ADR-013 §3). This
// table decides what you can SEE; auth decides what you can DO.
//
// Two mechanisms keep it honest, and both are needed:
//
//  1. Events (auth.role_granted / auth.role_revoked) apply changes as they
//     happen. Core NATS has no redelivery, so a dropped message is a real
//     possibility rather than a theoretical one.
//  2. A periodic full reconcile against auth-service asks the source of
//     truth outright. An event bus with no redelivery needs a floor under
//     how wrong its projection can get — the same reasoning
//     marginal/authverify's JWKS cache already uses.
//
// The blast radius of staleness is bounded to READS, and stated on the
// screens rather than hidden.
package spaceproj

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	spacerepo "marginal/document-service/internal/spacerepo/gen"
)

// The two topics DATA_MODEL.md §10 reserved for exactly this.
const (
	SubjectRoleGranted = "auth.role_granted"
	SubjectRoleRevoked = "auth.role_revoked"
)

const handleEventTimeout = 10 * time.Second

// ErrEmptySource refuses a reconcile that would revoke everyone at once.
// "auth has no memberships at all" is far more likely to be a broken call
// than a true statement, and adopting it costs every reader their access.
var ErrEmptySource = errors.New("spaceproj: refusing to apply an empty membership set")

// RolePayload mirrors auth-service's outbox.RolePayload field-for-field,
// kept as an independent definition rather than a shared module — the same
// choice notification-service already made for auth.user_registered, and
// for the same reason: a shared struct makes one service's refactor a
// compile error in another, which is the coupling events exist to avoid.
type RolePayload struct {
	UserID    uuid.UUID `json:"user_id"`
	SpaceID   uuid.UUID `json:"space_id"`
	Role      string    `json:"role,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
}

// wireEvent is the envelope the outbox publisher wraps every payload in.
type wireEvent struct {
	ID          uuid.UUID       `json:"id"`
	AggregateID uuid.UUID       `json:"aggregate_id"`
	Payload     json.RawMessage `json:"payload"`
}

// Membership is one row of the source of truth, as the reconcile reads it.
type Membership struct {
	UserID    uuid.UUID
	SpaceID   uuid.UUID
	Role      string
	GrantedAt time.Time
}

// Source is where the reconcile asks for the truth — an interface declared
// at its point of use, so a test needs no auth-service.
type Source interface {
	ListAllMemberships(ctx context.Context) ([]Membership, error)
}

type Projector struct {
	q    *spacerepo.Queries
	pool *pgxpool.Pool
}

func NewProjector(pool *pgxpool.Pool) *Projector {
	return &Projector{q: spacerepo.New(pool), pool: pool}
}

func pgUUID(id uuid.UUID) pgtype.UUID       { return pgtype.UUID{Bytes: id, Valid: true} }
func pgTime(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// Grant applies auth.role_granted.
func (p *Projector) Grant(ctx context.Context, e RolePayload) error {
	if err := p.q.UpsertSpaceMember(ctx, spacerepo.UpsertSpaceMemberParams{
		UserID: pgUUID(e.UserID), SpaceID: pgUUID(e.SpaceID),
		Role: e.Role, GrantedAt: pgTime(e.GrantedAt),
	}); err != nil {
		return fmt.Errorf("spaceproj: applying grant: %w", err)
	}
	return nil
}

// Revoke applies auth.role_revoked.
func (p *Projector) Revoke(ctx context.Context, e RolePayload) error {
	if err := p.q.DeleteSpaceMemberIfOlder(ctx, spacerepo.DeleteSpaceMemberIfOlderParams{
		UserID: pgUUID(e.UserID), SpaceID: pgUUID(e.SpaceID),
		GrantedAt: pgTime(e.GrantedAt),
	}); err != nil {
		return fmt.Errorf("spaceproj: applying revoke: %w", err)
	}
	return nil
}

// SpacesFor is the question every read asks: which spaces may this reader
// see. Empty means none, which is a real answer — a user in no space sees
// no pages, and returning "everything" for an empty set is how a filter
// becomes a no-op precisely when it matters most.
func (p *Projector) SpacesFor(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := p.q.SpacesForUser(ctx, pgUUID(userID))
	if err != nil {
		return nil, fmt.Errorf("spaceproj: reading spaces: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, uuid.UUID(r.Bytes))
	}
	return out, nil
}

// Reconcile replaces the whole projection from the source of truth.
//
// Truncate-and-load inside ONE transaction: a projection that is partly old
// and partly new is worse than one that is uniformly stale, because nothing
// about it can be trusted.
func (p *Projector) Reconcile(ctx context.Context, src Source) (int, error) {
	rows, err := src.ListAllMemberships(ctx)
	if err != nil {
		return 0, fmt.Errorf("spaceproj: asking for memberships: %w", err)
	}
	if len(rows) == 0 {
		return 0, ErrEmptySource
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("spaceproj: beginning reconcile: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := spacerepo.New(tx)
	if err := q.ClearSpaceMembers(ctx); err != nil {
		return 0, fmt.Errorf("spaceproj: clearing projection: %w", err)
	}
	for _, m := range rows {
		if err := q.UpsertSpaceMember(ctx, spacerepo.UpsertSpaceMemberParams{
			UserID: pgUUID(m.UserID), SpaceID: pgUUID(m.SpaceID),
			Role: m.Role, GrantedAt: pgTime(m.GrantedAt),
		}); err != nil {
			return 0, fmt.Errorf("spaceproj: loading projection: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("spaceproj: committing reconcile: %w", err)
	}
	return len(rows), nil
}

// Subscribe wires both topics onto p. Same shape as blockproj.Subscribe —
// a decode failure is logged and dropped rather than retried, because a
// payload this service cannot parse will not become parseable on a retry.
func Subscribe(nc *nats.Conn, p *Projector) (func() error, error) {
	handler := func(apply func(context.Context, RolePayload) error, subject string) nats.MsgHandler {
		return func(msg *nats.Msg) {
			var evt wireEvent
			if err := json.Unmarshal(msg.Data, &evt); err != nil {
				slog.Error("spaceproj: decoding envelope", "subject", subject, "err", err)
				return
			}
			var payload RolePayload
			if err := json.Unmarshal(evt.Payload, &payload); err != nil {
				slog.Error("spaceproj: decoding payload", "subject", subject, "event_id", evt.ID, "err", err)
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), handleEventTimeout)
			defer cancel()
			if err := apply(ctx, payload); err != nil {
				slog.Error("spaceproj: applying event", "subject", subject,
					"event_id", evt.ID, "user_id", payload.UserID, "err", err)
			}
		}
	}

	granted, err := nc.Subscribe(SubjectRoleGranted, handler(p.Grant, SubjectRoleGranted))
	if err != nil {
		return nil, err
	}
	revoked, err := nc.Subscribe(SubjectRoleRevoked, handler(p.Revoke, SubjectRoleRevoked))
	if err != nil {
		_ = granted.Unsubscribe()
		return nil, err
	}
	return func() error {
		if err := granted.Unsubscribe(); err != nil {
			_ = revoked.Unsubscribe()
			return err
		}
		return revoked.Unsubscribe()
	}, nil
}

// RunReconcile reconciles once at startup and then on a ticker.
//
// The startup pass matters most: this service may have been down while
// events were published, and core NATS delivers nothing to a subscriber
// that was not there. Without it, a restart is exactly when the projection
// is most likely to be wrong and least likely to notice.
func RunReconcile(ctx context.Context, p *Projector, src Source, every time.Duration) {
	run := func() {
		c, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		n, err := p.Reconcile(c, src)
		if err != nil {
			slog.Error("spaceproj: reconcile failed", "err", err)
			return
		}
		slog.Info("spaceproj: reconciled", "memberships", n)
	}
	run()

	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
