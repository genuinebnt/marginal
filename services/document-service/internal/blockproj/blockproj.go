// Package blockproj materialises docs.blocks/docs.page_links by
// consuming collab.ops_flushed (DATA_MODEL.md § collab.ops → docs.blocks)
// — collaboration-service's own database is the source of truth for a
// page's live block content; this is a read model, rebuilt from that
// event stream, never written to directly by any request handler.
//
// A projector, not a live editing model: it doesn't need
// collaboration-service's anchor/rope machinery to reconstruct a block's
// text correctly. Every real client in this repo only ever sends a
// Text op as "delete everything, insert everything" (the
// whole-block-replace strategy docs/api/collaboration.md documents,
// web/src/collab/useCollabPage.ts's own doc comment explains why) — so
// treating the most recent InsertText.text as a block's current full
// content reproduces the same end state a true anchor-resolving replay
// would, without needing that machinery duplicated here. Structural ops
// (documentcore.Op) hold a real documentcore.Page and apply via
// Page.Apply directly — RFC-001 §1's containment (Quote/Toggle/List/
// ListItem nesting) needs the same depth-first-order/cycle/container
// bookkeeping Page.Apply already implements and tests, so this package
// doesn't reimplement any of it a second time.
//
// Plain core NATS, not JetStream, same accepted gap as
// notification-service's auth.user_registered consumer: an event
// published while this consumer isn't running is lost, not queued. A
// page's projection can lag or (if this consumer was down since that
// page's last edit) miss a change entirely; docs/porting/PROGRESS.md
// records this as a known, deliberate limitation at this repo's scope,
// not a bug to route around with JetStream's added operational
// complexity.
package blockproj

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"marginal/documentcore"

	"marginal/document-service/internal/blockrepo/gen"
)

// SubjectOpsFlushed is the NATS subject collaboration-service's outbox
// poller publishes to (internal/opstore.OutboxEventOpAppended there) —
// mirrored independently here, same convention as
// notification-service/internal/notify's SubjectUserRegistered.
const SubjectOpsFlushed = "collab.ops_flushed"

// wireEvent is the envelope collaboration-service's poller actually
// publishes (internal/outbox.wireEvent there) — AggregateID is the page
// id; collab.outbox.payload is only ever the op itself; nothing in it
// names which page it belongs to on its own.
//
// Payload is json.RawMessage, not []byte: encoding/json base64-encodes a
// plain []byte field, which round-trips fine between two Go processes
// (both sides use the same package) but produces a wire message whose
// "payload" field is an opaque base64 string, not the readable nested
// JSON object it actually is — a trap for any non-Go consumer, a human
// inspecting the NATS message, or the future Rust port. json.RawMessage
// passes the bytes through verbatim instead.
type wireEvent struct {
	ID          uuid.UUID       `json:"id"`
	AggregateID uuid.UUID       `json:"aggregate_id"`
	Payload     json.RawMessage `json:"payload"`
}

// pageLinkPattern matches [[Page Title]] — RFC-003 §2's PageLink mark,
// scanned from plain block text rather than a real mark (internal/doctext
// has no mark storage yet — see docs/porting/PROGRESS.md); this is
// enough to build real backlinks now without waiting on marks to exist.
var pageLinkPattern = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// Projector holds one documentcore.Page per page touched since this
// process started, hydrated lazily from docs.blocks on first touch
// (never from collab.ops directly — this service has no access to that
// database, ADR-003). Safe for concurrent use; NATS delivers to one
// handler goroutine at a time per subscription, but a mutex costs
// nothing and removes any doubt.
type Projector struct {
	mu    sync.Mutex
	pool  *pgxpool.Pool
	pages map[uuid.UUID]*documentcore.Page
}

func New(pool *pgxpool.Pool) *Projector {
	return &Projector{pool: pool, pages: make(map[uuid.UUID]*documentcore.Page)}
}

// HandleEvent applies one collab.ops_flushed event to pageID's projection
// and persists the whole page's current blocks/links — see the package
// doc comment for why a full rewrite, not an incremental patch, is the
// right amount of simplicity here.
func (p *Projector) HandleEvent(ctx context.Context, pageID uuid.UUID, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	page, ok := p.pages[pageID]
	if !ok {
		loaded, err := p.loadPage(ctx, pageID)
		if err != nil {
			return fmt.Errorf("blockproj: loading existing state for %s: %w", pageID, err)
		}
		page = loaded
		p.pages[pageID] = page
	}

	var envelope struct {
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("blockproj: decoding op envelope: %w", err)
	}

	switch envelope.Scope {
	case "block":
		if err := applyBlockOp(page, payload); err != nil {
			return fmt.Errorf("blockproj: applying block op: %w", err)
		}
	case "text":
		var t struct {
			Block string `json:"block"`
			Op    struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"op"`
		}
		if err := json.Unmarshal(payload, &t); err != nil {
			return fmt.Errorf("blockproj: decoding text op: %w", err)
		}
		blockID, err := uuid.Parse(t.Block)
		if err != nil {
			return fmt.Errorf("blockproj: invalid block id %q: %w", t.Block, err)
		}
		applyTextOp(page, documentcore.BlockID(blockID), t.Op.Type, t.Op.Text)
	default:
		return fmt.Errorf("blockproj: unknown op scope %q", envelope.Scope)
	}

	return p.persist(ctx, pageID, page)
}

func (p *Projector) loadPage(ctx context.Context, pageID uuid.UUID) (*documentcore.Page, error) {
	q := blockrepo.New(p.pool)
	rows, err := q.ListBlocksForPage(ctx, toPgUUID(pageID))
	if err != nil {
		return nil, err
	}
	// title is unused — docs.pages.title (RenamePage) is the real source
	// of truth (see applyBlockOp's SetTitle case); this Page only ever
	// needs its Blocks.
	page := documentcore.NewPage(documentcore.PageID(pageID), "")
	// rows are already position-ordered (ListBlocksForPage's own ORDER
	// BY), and position is always written as this page's depth-first
	// order (persist, below) — appending in that same order reconstructs
	// Page.Blocks' own required invariant without re-sorting.
	page.Blocks = make([]documentcore.Block, 0, len(rows))
	for _, r := range rows {
		var kind documentcore.BlockKind
		if err := json.Unmarshal(r.Kind, &kind); err != nil {
			return nil, fmt.Errorf("decoding stored kind: %w", err)
		}
		var content documentcore.Content
		if err := json.Unmarshal(r.Content, &content); err != nil {
			return nil, fmt.Errorf("decoding stored content: %w", err)
		}
		page.Blocks = append(page.Blocks, documentcore.Block{
			ID:      documentcore.BlockID(fromPgUUID(r.ID)),
			Parent:  fromPgUUIDPtr(r.ParentID),
			Kind:    kind,
			Content: content,
		})
	}
	return &page, nil
}

// applyBlockOp decodes a "block"-scope payload directly via
// documentcore.UnmarshalOp (the same type-tagged envelope
// documentcore.MarshalOp produces; the extra "scope" field is simply
// ignored by encoding/json) and applies it via page.Apply itself — no
// second implementation of the block tree's own order/containment
// logic. Page.Apply's preconditions run again here too, unlike this
// package's earlier design (which skipped them, trusting collaboration-
// service's own authoritative check): harmless when they hold, which
// they always should since the op already committed upstream, and a
// strictly better failure mode when they somehow don't — a named, typed
// error instead of a silently wrong projection.
func applyBlockOp(page *documentcore.Page, payload []byte) error {
	op, err := documentcore.UnmarshalOp(payload)
	if err != nil {
		return err
	}
	if _, ok := op.(documentcore.SetTitle); ok {
		// Not projected — docs.pages.title (RenamePage) is already the
		// source of truth for a page's title; nothing here duplicates it.
		// (Applying it against this Page's own always-empty Title would
		// also just fail Page.Apply's own precondition check every time,
		// since nothing here ever keeps that shadow field in sync.)
		return nil
	}
	return page.Apply(op)
}

// applyTextOp treats the most recent InsertText's text as blockID's
// current full content — see the package doc comment for why this
// reproduces the correct end state without anchor resolution. DeleteText
// and NoOp leave the block's projected text untouched: DeleteText is
// always immediately followed by the InsertText that supplies the block's
// new full content (the whole-block-replace strategy), and NoOp carries
// no content change at all — treating either as "clear to empty" wiped a
// block's projected text to "" with no following event to correct it
// whenever the replace's insert half was itself empty, or the op really
// was a no-op. Only Text (not Marks) is ever touched here — a block that
// has acquired any mark switches to SetBlockContent for every future
// edit (web/src/collab/marks.ts's own doc comment), so a block still
// receiving character-level InsertText never has marks to begin with.
// An unknown block id (a redelivered event for a block deleted since) is
// silently ignored, matching can_apply's authorization having already
// run upstream.
func applyTextOp(page *documentcore.Page, blockID documentcore.BlockID, opType, text string) {
	if opType != "InsertText" {
		return
	}
	if i, ok := indexOfBlock(page, blockID); ok {
		page.Blocks[i].Content.Text = text
	}
}

func indexOfBlock(page *documentcore.Page, id documentcore.BlockID) (int, bool) {
	for i := range page.Blocks {
		if page.Blocks[i].ID == id {
			return i, true
		}
	}
	return -1, false
}

// blockPathLabel is docs.blocks.path's LTREE label for a block — same
// convention as internal/pages.pathLabel (a "b" prefix here, not "p",
// purely for readability when debugging a row; the two tables' paths
// never mix), since LTREE labels may only contain letters, digits, and
// underscores.
func blockPathLabel(id documentcore.BlockID) string {
	return "b" + strings.ReplaceAll(uuid.UUID(id).String(), "-", "")
}

// persist rewrites pageID's whole docs.blocks/docs.page_links projection
// from page — one transaction, delete-then-bulk-insert both tables (the
// package doc comment explains why a full rewrite is the right amount of
// simplicity for a rebuildable read model at this repo's scale). Each
// block's path is computed here, not carried on documentcore.Block
// itself — RFC-001 §1 "Persisted form is not the in-memory form":
// materialising the full ancestry is this projection's job, done in one
// forward pass since page.Blocks is already depth-first-ordered (a
// parent always appears before its own children).
func (p *Projector) persist(ctx context.Context, pageID uuid.UUID, page *documentcore.Page) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := blockrepo.New(tx)

	if err := q.ReplaceBlocksForPage(ctx, toPgUUID(pageID)); err != nil {
		return fmt.Errorf("clearing existing blocks: %w", err)
	}

	paths := make(map[documentcore.BlockID]string, len(page.Blocks))
	blockParams := make([]blockrepo.InsertBlockBatchParams, 0, len(page.Blocks))
	seenLinks := make(map[string]bool) // dedupes (block, title) within this one page — docs.page_links' UNIQUE constraint
	linkParams := make([]blockrepo.InsertPageLinkBatchParams, 0)
	for i, b := range page.Blocks {
		path := blockPathLabel(b.ID)
		if b.Parent != nil {
			path = paths[*b.Parent] + "." + path
		}
		paths[b.ID] = path

		kindJSON, err := json.Marshal(b.Kind)
		if err != nil {
			return fmt.Errorf("marshaling kind for block %s: %w", b.ID, err)
		}
		contentJSON, err := json.Marshal(b.Content)
		if err != nil {
			return fmt.Errorf("marshaling content for block %s: %w", b.ID, err)
		}
		blockParams = append(blockParams, blockrepo.InsertBlockBatchParams{
			ID: toPgUUID(uuid.UUID(b.ID)), PageID: toPgUUID(pageID), ParentID: toPgUUIDPtr(b.Parent),
			Path: path, Position: int32(i), Kind: kindJSON, Content: contentJSON,
		})

		for _, title := range extractLinkTitles(b.Content.Text) {
			key := b.ID.String() + "\x00" + title
			if seenLinks[key] {
				continue
			}
			seenLinks[key] = true
			linkID := uuid.Must(uuid.NewV7())
			linkParams = append(linkParams, blockrepo.InsertPageLinkBatchParams{
				ID: toPgUUID(linkID), FromPage: toPgUUID(pageID), FromBlock: toPgUUID(uuid.UUID(b.ID)),
				TargetTitle: title, TargetTitleForLookup: title,
			})
		}
	}

	if len(blockParams) > 0 {
		var firstErr error
		q.InsertBlockBatch(ctx, blockParams).Exec(func(_ int, err error) {
			if err != nil && firstErr == nil {
				firstErr = err
			}
		})
		if firstErr != nil {
			return fmt.Errorf("inserting blocks: %w", firstErr)
		}
	}

	if err := q.ReplacePageLinksForPage(ctx, toPgUUID(pageID)); err != nil {
		return fmt.Errorf("clearing existing page links: %w", err)
	}
	if len(linkParams) > 0 {
		var firstErr error
		q.InsertPageLinkBatch(ctx, linkParams).Exec(func(_ int, err error) {
			if err != nil && firstErr == nil {
				firstErr = err
			}
		})
		if firstErr != nil {
			return fmt.Errorf("inserting page links: %w", firstErr)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func extractLinkTitles(text string) []string {
	matches := pageLinkPattern.FindAllStringSubmatch(text, -1)
	titles := make([]string, len(matches))
	for i, m := range matches {
		titles[i] = m[1]
	}
	return titles
}

func toPgUUID(id uuid.UUID) pgtype.UUID   { return pgtype.UUID{Bytes: id, Valid: true} }
func fromPgUUID(id pgtype.UUID) uuid.UUID { return uuid.UUID(id.Bytes) }

func toPgUUIDPtr(id *documentcore.BlockID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{Valid: false}
	}
	return toPgUUID(uuid.UUID(*id))
}

func fromPgUUIDPtr(id pgtype.UUID) *documentcore.BlockID {
	if !id.Valid {
		return nil
	}
	b := documentcore.BlockID(fromPgUUID(id))
	return &b
}

// handleEventTimeout bounds one HandleEvent call — the DB write inside it
// must not hang the NATS delivery goroutine forever on a stuck connection;
// see the package doc comment on why a missed/late event here is already
// an accepted gap at this repo's scope, so this timeout errs toward "give
// up and let the next event resync via a full loadPage," not toward
// retrying indefinitely.
const handleEventTimeout = 10 * time.Second

// Subscribe registers a NATS subscription that calls proj.HandleEvent for
// each collab.ops_flushed message. Returns an unsubscribe func. See the
// package doc comment for the plain-core-NATS gap this inherits.
func Subscribe(nc *nats.Conn, proj *Projector) (unsubscribe func() error, err error) {
	sub, err := nc.Subscribe(SubjectOpsFlushed, func(msg *nats.Msg) {
		var evt wireEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			slog.Error("blockproj: decoding collab.ops_flushed envelope", "err", err)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), handleEventTimeout)
		defer cancel()
		if err := proj.HandleEvent(ctx, evt.AggregateID, evt.Payload); err != nil {
			slog.Error("blockproj: handling event", "page_id", evt.AggregateID, "event_id", evt.ID, "err", err)
		}
	})
	if err != nil {
		return nil, err
	}
	return sub.Unsubscribe, nil
}
