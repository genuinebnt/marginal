package wsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	collabrepo "marginal/collaboration-service/internal/collabrepo/gen"
	"marginal/collaboration-service/internal/mention"
	"marginal/collaboration-service/internal/roles"
)

// MentionEvent is the topic a mention travels on. Named here rather than
// in notification-service because the producer owns the contract
// (DATA_MODEL.md § 10) — see docs/api/notifications.md.
const MentionEvent = "collab.comment_mentioned"

// MemberLister is the mention half of roles.Resolver, declared where it is
// used (CLOUD_PORTABILITY.md's rule) so this file's tests need no gRPC.
type MemberLister interface {
	Members(ctx context.Context, pageID, actorID uuid.UUID) ([]roles.Member, error)
}

// mentionPayload is what a mention notification is made of.
//
// § 20's rule, and the reason this carries ids rather than text: "A
// notification is a pointer to an anchor, never a copy of the text. Open it
// a week later and it still lands on the right words, wherever they moved."
// The comment body is NOT here. The inbox reads it back through the
// comments API, which resolves the anchors against the live rope and says
// so when they no longer resolve — a copy taken now would be a quotation of
// a page that has since changed, presented as if it were current.
type mentionPayload struct {
	PageID    uuid.UUID `json:"page_id"`
	BlockID   uuid.UUID `json:"block_id"`
	ThreadID  uuid.UUID `json:"thread_id"`
	CommentID uuid.UUID `json:"comment_id"`
	// Who did the mentioning, and who is being told about it.
	ActorID uuid.UUID `json:"actor_id"`
	UserID  uuid.UUID `json:"user_id"`
}

// resolveMentions turns a comment body into the set of people to notify.
//
// Returns nil for the overwhelmingly common case of a comment with no @ in
// it at all, WITHOUT any network call — the parse is pure and runs first,
// so the cost of the feature is paid only by comments that use it.
func resolveMentions(
	ctx context.Context, lister MemberLister, body string, pageID, author uuid.UUID,
) ([]uuid.UUID, error) {
	handles := mention.Parse(body)
	if len(handles) == 0 {
		return nil, nil
	}
	members, err := lister.Members(ctx, pageID, author)
	if err != nil {
		return nil, err
	}
	byHandle := make(map[string]uuid.UUID, len(members))
	for _, m := range members {
		// First writer wins. Two people whose display names normalise to
		// the same handle is a real possibility, and notifying BOTH would
		// make @ada mean "everyone called Ada" — worse than notifying the
		// wrong one, because it cannot be corrected by typing more.
		if h := mention.Normalize(m.DisplayName); h != "" {
			if _, taken := byHandle[h]; !taken {
				byHandle[h] = m.UserID
			}
		}
	}
	var out []uuid.UUID
	for _, h := range handles {
		id, ok := byHandle[h]
		// A handle nobody answers to is not an error. People type names
		// that are not accounts, and refusing the comment over it would
		// make a typo cost somebody their remark.
		if !ok {
			continue
		}
		// § 20 WHAT NEVER NOTIFIES: "Your own ops." Mentioning yourself is
		// a note to self, and a badge for something you just typed is the
		// definition of noise wearing one.
		if id == author {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

// writeMentionEvents inserts one outbox row per person mentioned, in the
// caller's transaction — the same transaction the comment itself is written
// in. That is the whole point of the outbox: a mention cannot be delivered
// for a comment that failed to save, and a comment cannot save while its
// mentions quietly go nowhere.
func writeMentionEvents(
	ctx context.Context, tx pgx.Tx, users []uuid.UUID, p mentionPayload,
) error {
	q := collabrepo.New(tx)
	for _, u := range users {
		p.UserID = u
		payload, err := json.Marshal(p)
		if err != nil {
			return fmt.Errorf("marshalling mention payload: %w", err)
		}
		if _, err := q.InsertOutboxEvent(ctx, collabrepo.InsertOutboxEventParams{
			ID:          pgID(uuid.Must(uuid.NewV7())),
			AggregateID: pgID(p.PageID),
			EventType:   MentionEvent,
			Payload:     payload,
		}); err != nil {
			return fmt.Errorf("writing mention outbox row: %w", err)
		}
	}
	return nil
}

// logMentionFailure is how a failure to RESOLVE (not to write) is handled.
//
// Resolution needs document-service and auth-service; the comment needs
// neither. Failing the write because a third service was slow would mean an
// unrelated outage eats what somebody typed, so the comment is saved and
// the mention is dropped — loudly, because a silently dropped notification
// is indistinguishable from one nobody bothered to send.
func logMentionFailure(pageID uuid.UUID, err error) {
	slog.Error("wsapi: resolving mentions (comment saved, mention dropped)",
		"page_id", pageID, "err", err)
}
