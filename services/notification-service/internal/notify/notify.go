// Package notify is notification-service's one real feature: consume
// auth.user_registered (DATA_MODEL.md §10 — the one event topic Track 1
// can actually produce; see docs/porting/PROGRESS.md's scope-decision
// entry for why every other notification-worthy topic needs a feature
// that isn't in scope) and persist a welcome notification, idempotently.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"marginal/notification-service/internal/notifyrepo/gen"
)

// UserRegisteredEvent is auth.user_registered's payload shape —
// auth-service's internal/outbox package publishes exactly this (kept as
// two independent, matching struct definitions rather than a shared
// module, per this repo's established convention when only two small,
// asymmetric roles — publisher, subscriber — need the same wire shape;
// see docs/porting/PROGRESS.md's api-gateway entry for why a shared
// module was worth it there but isn't here).
type UserRegisteredEvent struct {
	UserID      uuid.UUID `json:"user_id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
}

// KindWelcome is the only notification kind this repo produces.
const KindWelcome = "welcome"

type Notification struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	SourceEventID uuid.UUID
	Kind          string
	Message       string
	ReadAt        *time.Time
	CreatedAt     time.Time
}

// Repo is notify's only port — small, declared at its point of use
// (CLOUD_PORTABILITY.md), and unlike document-service's now-deleted
// pages.Repo (one implementation, no test double), this one has a real
// second implementation (fakeRepo, notify_test.go) exercising
// HandleUserRegistered without Postgres.
//
// PORT-NOTE: every call site here resolves its Repo at compile time —
// nothing ever holds a Repo value and switches implementations at
// runtime. A Rust port should express this as a generic type parameter
// bound (`fn handle_user_registered<R: Repo>(repo: &R, ...)`), not a
// `dyn Trait` object — the dynamic dispatch a trait object buys has no
// use here, and Go's own interface value already costs an allocation +
// indirection this codebase pays for free under the GC; Rust doesn't
// need to inherit that cost just because Go's version looks like this.
type Repo interface {
	// Create persists a notification, idempotent on sourceEventID —
	// redelivering the same event must not create a duplicate. Returns
	// (false, nil) on a duplicate, not an error.
	Create(ctx context.Context, userID, sourceEventID uuid.UUID, kind, message string) (created bool, err error)
	ListForUser(ctx context.Context, userID uuid.UUID, limit int32) ([]Notification, error)
}

type PostgresRepo struct {
	q *notifyrepo.Queries
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{q: notifyrepo.New(pool)}
}

func (r *PostgresRepo) Create(ctx context.Context, userID, sourceEventID uuid.UUID, kind, message string) (bool, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return false, fmt.Errorf("notify: create: generating id: %w", err)
	}
	_, err = r.q.InsertNotification(ctx, notifyrepo.InsertNotificationParams{
		ID:            pgtype.UUID{Bytes: id, Valid: true},
		UserID:        pgtype.UUID{Bytes: userID, Valid: true},
		SourceEventID: pgtype.UUID{Bytes: sourceEventID, Valid: true},
		Kind:          kind,
		Message:       message,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING: this sourceEventID was already consumed
		// by an earlier delivery of the same event — same convention as
		// opstore.Append's dedup-on-id handling.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("notify: create: %w", err)
	}
	return true, nil
}

func (r *PostgresRepo) ListForUser(ctx context.Context, userID uuid.UUID, limit int32) ([]Notification, error) {
	rows, err := r.q.ListNotificationsForUser(ctx, notifyrepo.ListNotificationsForUserParams{
		UserID: pgtype.UUID{Bytes: userID, Valid: true},
		Limit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("notify: list for user: %w", err)
	}
	out := make([]Notification, len(rows))
	for i, row := range rows {
		out[i] = notificationFromRow(row)
	}
	return out, nil
}

func notificationFromRow(row notifyrepo.NotifyNotification) Notification {
	n := Notification{
		ID:            row.ID.Bytes,
		UserID:        row.UserID.Bytes,
		SourceEventID: row.SourceEventID.Bytes,
		Kind:          row.Kind,
		Message:       row.Message,
		CreatedAt:     row.CreatedAt.Time,
	}
	if row.ReadAt.Valid {
		t := row.ReadAt.Time
		n.ReadAt = &t
	}
	return n
}

// HandleUserRegistered decodes an auth.user_registered payload and
// persists the welcome notification it implies. eventID is the
// publishing outbox row's own id — the dedup key.
func HandleUserRegistered(ctx context.Context, repo Repo, eventID uuid.UUID, payload []byte) error {
	var evt UserRegisteredEvent
	if err := json.Unmarshal(payload, &evt); err != nil {
		return fmt.Errorf("notify: decoding auth.user_registered payload: %w", err)
	}
	message := fmt.Sprintf("Welcome to Marginal, %s!", evt.DisplayName)
	_, err := repo.Create(ctx, evt.UserID, eventID, KindWelcome, message)
	return err
}
