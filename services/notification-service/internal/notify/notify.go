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

// KindWelcome is the notification a registration produces.
const KindWelcome = "welcome"

// KindMention is what an @handle in a comment produces (v3.3.0).
//
// Second kind, and the first whose content is a POINTER: nothing about a
// mention is copied here, because every part of it belongs to a service
// that is allowed to change it — see MentionPointer.
const KindMention = "mention"

// KindInvite is a space invitation (v3.3.0, § 20's ACCEPT/DECLINE row).
//
// Pointer-shaped for the same reason a mention is: the space's name, the
// inviter's name and the role are all owned elsewhere and may change
// between the invitation and the reading of it. What is stored is the
// invitation's id, which is also the thing the recipient answers with.
const KindInvite = "invite"

// InviteEvent is the subject auth-service publishes invitations on.
const InviteEvent = "auth.member_invited"

// InvitePointer is auth.member_invited's payload.
type InvitePointer struct {
	InvitationID uuid.UUID `json:"invitation_id"`
	UserID       uuid.UUID `json:"user_id"`
	SpaceID      uuid.UUID `json:"space_id"`
	Role         string    `json:"role"`
	InvitedBy    uuid.UUID `json:"invited_by"`
}

// MentionEvent is the NATS subject collaboration-service's outbox poller
// publishes mentions to. Mirrored independently there (wsapi.MentionEvent),
// the same two-matching-definitions convention SubjectUserRegistered
// already uses.
const MentionEvent = "collab.comment_mentioned"

// MentionPointer is collab.comment_mentioned's payload: ids, and nothing
// else.
//
// § 20 states the rule the absence of a `body` field here enforces: "A
// notification is a pointer to an anchor, never a copy of the text. Open it
// a week later and it still lands on the right words, wherever they moved."
// A reader resolves the thread through the comments API, which reports
// where the anchors point NOW and says plainly when they no longer resolve.
type MentionPointer struct {
	PageID    uuid.UUID `json:"page_id"`
	BlockID   uuid.UUID `json:"block_id"`
	ThreadID  uuid.UUID `json:"thread_id"`
	CommentID uuid.UUID `json:"comment_id"`
	ActorID   uuid.UUID `json:"actor_id"`
	UserID    uuid.UUID `json:"user_id"`
}

type Notification struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	SourceEventID uuid.UUID
	Kind          string
	// Empty for every pointer-shaped kind. A mention's sentence is
	// assembled by the reader from ids, not stored pre-rendered here.
	Message string
	// Who acted. Nil for a kind nobody caused (welcome).
	ActorID *uuid.UUID
	// The pointer itself, passed through verbatim. Shaped by Kind:
	// MentionPointer for KindMention, absent for KindWelcome.
	Pointer   json.RawMessage
	ReadAt    *time.Time
	CreatedAt time.Time
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
	// CreatePointer persists a notification whose content is a pointer
	// rather than a sentence. Separate from Create rather than a fifth
	// nullable argument to it: the two kinds have disjoint fields, and one
	// call taking both would let a caller write a row that is neither.
	CreatePointer(ctx context.Context, userID, sourceEventID, actorID uuid.UUID, kind string, pointer json.RawMessage) (created bool, err error)
	ListForUser(ctx context.Context, userID uuid.UUID, limit int32) ([]Notification, error)
	// MarkRead clears one row. Scoped by user as well as id — an id alone
	// would be a bearer token for someone else's inbox. Reports how many
	// rows it changed, so "already read" is 0 rather than an error.
	MarkRead(ctx context.Context, userID, id uuid.UUID) (int64, error)
	// MarkAllRead is § 20's one bulk action. An inbox is cleared by acting
	// on it, and "acknowledge everything" is itself an act.
	MarkAllRead(ctx context.Context, userID uuid.UUID) (int64, error)
	// CountUnread backs the bell's badge, which is drawn on every screen and
	// so must not be a length over a limited list.
	CountUnread(ctx context.Context, userID uuid.UUID) (int64, error)
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

func (r *PostgresRepo) CreatePointer(
	ctx context.Context, userID, sourceEventID, actorID uuid.UUID, kind string, pointer json.RawMessage,
) (bool, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return false, fmt.Errorf("notify: create pointer: generating id: %w", err)
	}
	_, err = r.q.InsertPointerNotification(ctx, notifyrepo.InsertPointerNotificationParams{
		ID:            pgtype.UUID{Bytes: id, Valid: true},
		UserID:        pgtype.UUID{Bytes: userID, Valid: true},
		SourceEventID: pgtype.UUID{Bytes: sourceEventID, Valid: true},
		Kind:          kind,
		ActorID:       pgtype.UUID{Bytes: actorID, Valid: true},
		Pointer:       pointer,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("notify: create pointer: %w", err)
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

func (r *PostgresRepo) MarkRead(ctx context.Context, userID, id uuid.UUID) (int64, error) {
	n, err := r.q.MarkNotificationRead(ctx, notifyrepo.MarkNotificationReadParams{
		ID:     pgtype.UUID{Bytes: id, Valid: true},
		UserID: pgtype.UUID{Bytes: userID, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("notify: mark read: %w", err)
	}
	return n, nil
}

func (r *PostgresRepo) MarkAllRead(ctx context.Context, userID uuid.UUID) (int64, error) {
	n, err := r.q.MarkAllNotificationsRead(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("notify: mark all read: %w", err)
	}
	return n, nil
}

func (r *PostgresRepo) CountUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	n, err := r.q.CountUnreadForUser(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("notify: count unread: %w", err)
	}
	return n, nil
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
	if row.ActorID.Valid {
		a := uuid.UUID(row.ActorID.Bytes)
		n.ActorID = &a
	}
	n.Pointer = row.Pointer
	return n
}

// HandleMention decodes a collab.comment_mentioned payload and persists the
// mention it points at. eventID is the publishing outbox row's own id — the
// dedup key, exactly as for a registration.
//
// The payload is stored VERBATIM rather than re-marshalled from the decoded
// struct: a field collaboration-service adds later must survive this hop
// even before this service knows what it means. Only the two fields this
// service must act on — who it is for, and who acted — are read out.
func HandleMention(ctx context.Context, repo Repo, eventID uuid.UUID, payload []byte) error {
	var p MentionPointer
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("notify: decoding %s payload: %w", MentionEvent, err)
	}
	if p.UserID == uuid.Nil {
		// Refused rather than stored against the nil user, where it would
		// be invisible to everyone and countable by nobody.
		return fmt.Errorf("notify: %s carried no user_id", MentionEvent)
	}
	_, err := repo.CreatePointer(ctx, p.UserID, eventID, p.ActorID, KindMention, payload)
	return err
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

// HandleInvite persists a space invitation. Same shape as HandleMention,
// same dedup key, same verbatim payload — the only thing that differs is
// which subject it came from and which kind it is stored as.
func HandleInvite(ctx context.Context, repo Repo, eventID uuid.UUID, payload []byte) error {
	var p InvitePointer
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("notify: decoding %s payload: %w", InviteEvent, err)
	}
	if p.UserID == uuid.Nil {
		return fmt.Errorf("notify: %s carried no user_id", InviteEvent)
	}
	_, err := repo.CreatePointer(ctx, p.UserID, eventID, p.InvitedBy, KindInvite, payload)
	return err
}
