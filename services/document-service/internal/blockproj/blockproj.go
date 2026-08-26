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
// (documentcore.Op) are decoded and applied via the shared documentcore
// module directly — no second implementation of Page.Apply.
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
	"sync"

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
type wireEvent struct {
	ID          uuid.UUID `json:"id"`
	AggregateID uuid.UUID `json:"aggregate_id"`
	Payload     []byte    `json:"payload"`
}

// pageLinkPattern matches [[Page Title]] — RFC-003 §2's PageLink mark,
// scanned from plain block text rather than a real mark (internal/doctext
// has no mark storage yet — see docs/porting/PROGRESS.md); this is
// enough to build real backlinks now without waiting on marks to exist.
var pageLinkPattern = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

type blockState struct {
	kind documentcore.BlockKind
	text string
}

type pageState struct {
	order  []documentcore.BlockID
	blocks map[documentcore.BlockID]*blockState
}

// Projector holds one pageState per page touched since this process
// started, hydrated lazily from docs.blocks on first touch (never from
// collab.ops directly — this service has no access to that database,
// ADR-003). Safe for concurrent use; NATS delivers to one handler
// goroutine at a time per subscription, but a mutex costs nothing and
// removes any doubt.
type Projector struct {
	mu    sync.Mutex
	pool  *pgxpool.Pool
	pages map[uuid.UUID]*pageState
}

func New(pool *pgxpool.Pool) *Projector {
	return &Projector{pool: pool, pages: make(map[uuid.UUID]*pageState)}
}

// HandleEvent applies one collab.ops_flushed event to pageID's projection
// and persists the whole page's current blocks/links — see the package
// doc comment for why a full rewrite, not an incremental patch, is the
// right amount of simplicity here.
func (p *Projector) HandleEvent(ctx context.Context, pageID uuid.UUID, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ps, ok := p.pages[pageID]
	if !ok {
		loaded, err := p.loadPageState(ctx, pageID)
		if err != nil {
			return fmt.Errorf("blockproj: loading existing state for %s: %w", pageID, err)
		}
		ps = loaded
		p.pages[pageID] = ps
	}

	var envelope struct {
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("blockproj: decoding op envelope: %w", err)
	}

	switch envelope.Scope {
	case "block":
		if err := applyBlockOp(ps, payload); err != nil {
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
		applyTextOp(ps, documentcore.BlockID(blockID), t.Op.Type, t.Op.Text)
	default:
		return fmt.Errorf("blockproj: unknown op scope %q", envelope.Scope)
	}

	return p.persist(ctx, pageID, ps)
}

func (p *Projector) loadPageState(ctx context.Context, pageID uuid.UUID) (*pageState, error) {
	q := blockrepo.New(p.pool)
	rows, err := q.ListBlocksForPage(ctx, toPgUUID(pageID))
	if err != nil {
		return nil, err
	}
	ps := &pageState{blocks: make(map[documentcore.BlockID]*blockState, len(rows))}
	for _, r := range rows {
		var kind documentcore.BlockKind
		if err := json.Unmarshal(r.Kind, &kind); err != nil {
			return nil, fmt.Errorf("decoding stored kind: %w", err)
		}
		var content struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(r.Content, &content); err != nil {
			return nil, fmt.Errorf("decoding stored content: %w", err)
		}
		id := documentcore.BlockID(fromPgUUID(r.ID))
		ps.order = append(ps.order, id)
		ps.blocks[id] = &blockState{kind: kind, text: content.Text}
	}
	return ps, nil
}

// applyBlockOp decodes a "block"-scope payload directly via
// documentcore.UnmarshalOp (the same type-tagged envelope
// documentcore.MarshalOp produces; the extra "scope" field is simply
// ignored by encoding/json) and applies it to ps's order/blocks —
// mirroring documentcore.Page.Apply's own switch, but without its
// precondition checks: those already ran, authoritatively, in
// collaboration-service before this op was ever committed. A projector's
// job is to reflect current state, not re-validate it.
func applyBlockOp(ps *pageState, payload []byte) error {
	op, err := documentcore.UnmarshalOp(payload)
	if err != nil {
		return err
	}
	switch op := op.(type) {
	case documentcore.InsertBlock:
		insertAfter(ps, op.ID, op.After)
		ps.blocks[op.ID] = &blockState{kind: op.Kind, text: op.Content.Text}
	case documentcore.DeleteBlock:
		removeFromOrder(ps, op.Tombstone.ID)
		delete(ps.blocks, op.Tombstone.ID)
	case documentcore.SetBlockKind:
		if b, ok := ps.blocks[op.ID]; ok {
			b.kind = op.To
		}
	case documentcore.SetBlockContent:
		if b, ok := ps.blocks[op.Block]; ok {
			b.text = op.Content.Text
		}
	case documentcore.MoveBlock:
		removeFromOrder(ps, op.ID)
		insertAfter(ps, op.ID, op.To)
	case documentcore.SetTitle:
		// Not projected — docs.pages.title (RenamePage) is already the
		// source of truth for a page's title; nothing here duplicates it.
	}
	return nil
}

// applyTextOp treats the most recent InsertText's text as blockID's
// current full content — see the package doc comment for why this
// reproduces the correct end state without anchor resolution. An unknown
// block id (a redelivered event for a block deleted since) is silently
// ignored, matching can_apply's authorization having already run upstream.
func applyTextOp(ps *pageState, blockID documentcore.BlockID, opType, text string) {
	b, ok := ps.blocks[blockID]
	if !ok {
		return
	}
	if opType == "InsertText" {
		b.text = text
	} else {
		b.text = ""
	}
}

func indexOf(ps *pageState, id documentcore.BlockID) int {
	for i, x := range ps.order {
		if x == id {
			return i
		}
	}
	return -1
}

func removeFromOrder(ps *pageState, id documentcore.BlockID) {
	if i := indexOf(ps, id); i >= 0 {
		ps.order = append(ps.order[:i], ps.order[i+1:]...)
	}
}

// insertAfter places id immediately after afterID's current position (nil
// after means the document start) — mirrors documentcore.Page's own
// After-pointer semantics for InsertBlock/MoveBlock.
func insertAfter(ps *pageState, id documentcore.BlockID, after *documentcore.BlockID) {
	idx := 0
	if after != nil {
		if i := indexOf(ps, *after); i >= 0 {
			idx = i + 1
		}
	}
	ps.order = append(ps.order, documentcore.BlockID{})
	copy(ps.order[idx+1:], ps.order[idx:])
	ps.order[idx] = id
}

// persist rewrites pageID's whole docs.blocks/docs.page_links projection
// from ps — one transaction, delete-then-bulk-insert both tables (the
// package doc comment explains why a full rewrite is the right amount of
// simplicity for a rebuildable read model at this repo's scale).
func (p *Projector) persist(ctx context.Context, pageID uuid.UUID, ps *pageState) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := blockrepo.New(tx)

	if err := q.ReplaceBlocksForPage(ctx, toPgUUID(pageID)); err != nil {
		return fmt.Errorf("clearing existing blocks: %w", err)
	}

	blockParams := make([]blockrepo.InsertBlockBatchParams, 0, len(ps.order))
	seenLinks := make(map[string]bool) // dedupes (block, title) within this one page — docs.page_links' UNIQUE constraint
	linkParams := make([]blockrepo.InsertPageLinkBatchParams, 0)
	for i, id := range ps.order {
		b := ps.blocks[id]
		kindJSON, err := json.Marshal(b.kind)
		if err != nil {
			return fmt.Errorf("marshaling kind for block %s: %w", id, err)
		}
		contentJSON, err := json.Marshal(documentcore.Content{Text: b.text})
		if err != nil {
			return fmt.Errorf("marshaling content for block %s: %w", id, err)
		}
		blockParams = append(blockParams, blockrepo.InsertBlockBatchParams{
			ID: toPgUUID(uuid.UUID(id)), PageID: toPgUUID(pageID), Position: int32(i), Kind: kindJSON, Content: contentJSON,
		})

		for _, title := range extractLinkTitles(b.text) {
			key := id.String() + "\x00" + title
			if seenLinks[key] {
				continue
			}
			seenLinks[key] = true
			linkID := uuid.Must(uuid.NewV7())
			linkParams = append(linkParams, blockrepo.InsertPageLinkBatchParams{
				ID: toPgUUID(linkID), FromPage: toPgUUID(pageID), FromBlock: toPgUUID(uuid.UUID(id)),
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
		if err := proj.HandleEvent(context.Background(), evt.AggregateID, evt.Payload); err != nil {
			slog.Error("blockproj: handling event", "page_id", evt.AggregateID, "event_id", evt.ID, "err", err)
		}
	})
	if err != nil {
		return nil, err
	}
	return sub.Unsubscribe, nil
}
