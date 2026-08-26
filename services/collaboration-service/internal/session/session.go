// Package session is one page's live editing session — ARCHITECTURE.md §4's
// "Request Flow — Live Editing," the piece that actually drives a client's
// op through can_apply → apply → WAL (durable ack) → broadcast → the
// batched Postgres flush (internal/flush), and that reconstructs that
// state on open by replaying collab.ops (the source of truth) plus
// reconciling against any local WAL records a crash left un-flushed.
//
// A Session holds RFC-002 §2's two ISA tiers reconciled into one system,
// per docs/architecture/DATA_MODEL.md's collab.ops → docs.blocks note: a
// documentcore.Page for block structure (insert/delete/reorder/kind), and
// one doctext.Text live rope per block for that block's own character
// content — both driven through the same pageop.Op union, the same WAL,
// the same flush pipeline, and the same broadcast to subscribers. There is
// no longer a whole-page flat rope; a "page" is its Page plus its blocks'
// ropes.
//
// "Session open is a replay, and it reads only this service's own
// database" (ARCHITECTURE.md §4) — no snapshot system, no dependency on
// document-service. Two cheaper things come before a snapshot system ever
// would (per that same section): keeping a session warm after the last
// client leaves, and plain replay-from-zero on a cold start. This package
// only builds the second; the first (idle-eviction / keep-warm timers) is
// a memory-bound optimization this repo's demo scale doesn't need yet —
// Manager keeps every opened Session until CloseAll, deliberately, not by
// oversight.
package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/doctext"
	"marginal/collaboration-service/internal/flush"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/opstore"
	"marginal/collaboration-service/internal/pageop"
	"marginal/collaboration-service/internal/wal"
)

// ErrDenied is can_apply's rejection (RFC-002 §5). can_apply always
// returns true at today's single-tenant scope — nothing in this repo
// exercises a real denial — but the chokepoint itself is real and
// injectable (CanApplyFunc), so workspaces/RBAC later become "make this
// function actually check something" rather than threading authorization
// through every mutation path retroactively.
var ErrDenied = errors.New("session: op denied by can_apply")

// ErrUnknownBlock is a Text op naming a block the session's Page doesn't
// currently have — either a stale client (the block was deleted by
// another actor since) or a genuinely malformed op.
var ErrUnknownBlock = errors.New("session: text op names an unknown block")

// CanApplyFunc is RFC-002 §5's one auditable authorization chokepoint.
// Every op passes through it before touching the page or any block's rope.
type CanApplyFunc func(op pageop.Op, actorID uuid.UUID, actorKind oplog.ActorKind) bool

func allowAll(pageop.Op, uuid.UUID, oplog.ActorKind) bool { return true }

// CommitResult is what every client learns about a just-committed op: the
// op itself, and — only when that op was a pageop.Text (a character edit
// scoped to one block) — that block's current boundary anchors (nil if
// the block is now empty). Boundaries is unset (nil) for a pageop.Block
// op: structural ops don't need a rune-offset anchor to build their next
// op, only a block's own live text does (doctext.Text.Boundaries's own
// doc comment explains why).
type CommitResult struct {
	Op         oplog.LoggedOp
	Boundaries *anchor.AnchorRange
}

// Subscriber receives every op committed to a Session except the one it
// submitted itself (that op's ack is ApplyClientOp's own return value,
// not a Deliver call) — the transport layer (a WebSocket connection, in
// this repo's scope) is what implements this. DeliverPresence fires when
// another actor's *first* connection joins, or their *last* connection
// leaves — never once per tab, so two tabs from the same person don't
// look like two people (see Subscribe/unsubscribe's own bookkeeping).
// DeliverCursor fires on every other actor's cursor move (see SetCursor).
type Subscriber interface {
	Deliver(r CommitResult)
	DeliverPresence(e PresenceEvent)
	DeliverCursor(e CursorEvent)
}

// PresenceEvent is a join or leave — Joined false means "left."
type PresenceEvent struct {
	ActorID uuid.UUID
	Joined  bool
}

// CursorEvent is one actor's current caret/selection: which block (nil
// means "not focused in any block right now" — the actor blurred, or
// hasn't clicked in yet — Start/End are meaningless in that case and
// omitted from the wire), and a start/end rune-offset range into that
// block's live text (Start == End is a plain caret, not a selection).
// Deliberately not persisted anywhere — never touches the WAL, page, or
// blocks — the same ephemeral, last-write-wins-per-actor treatment
// PresenceEvent already gets, for the same reason: this is where someone
// is *right now*, not a fact worth reconstructing on replay.
type CursorEvent struct {
	ActorID uuid.UUID
	BlockID *documentcore.BlockID
	Start   int
	End     int
}

// BlockSnapshot is one block's current live state, as a connecting client
// needs to render and then edit it: identity, kind, Marks (documentcore.Op's
// own wire shape — a client applies them against Text the same way a
// SetBlockContent/InsertBlock op's own Content.Marks would), and — unlike
// documentcore.Block.Content.Text, which this snapshot deliberately does
// not use — the block's *live* text (its rope's current string, which can
// differ from whatever Content.Text an op last recorded) plus that rope's
// current boundary anchors for building the client's next Text op. Marks
// themselves only ever change via a SetBlockContent/InsertBlock op (never
// a Text op — session.go's own syncBlockContent doesn't touch them), so
// reading them from s.page.Blocks here, alongside the rope's live text, is
// correct: nothing else in this session ever changes them out from under it.
type BlockSnapshot struct {
	ID         documentcore.BlockID   `json:"id"`
	Kind       documentcore.BlockKind `json:"kind"`
	Text       string                 `json:"text"`
	Marks      []documentcore.Mark    `json:"marks,omitempty"`
	Boundaries *anchor.AnchorRange    `json:"boundaries,omitempty"`
}

// Snapshot is the whole page as of right now: title, ordered blocks, and
// each block's live text — what a client reconnecting to an already
// non-empty page needs to render before it can send its first op
// (docs/api/collaboration.md's "snapshot" frame).
type Snapshot struct {
	PageID documentcore.PageID `json:"page_id"`
	Title  string              `json:"title"`
	Blocks []BlockSnapshot     `json:"blocks"`
}

// Session is one page's live state: a documentcore.Page for block
// structure, one doctext.Text per block for that block's live content,
// the durability pipeline (internal/wal + internal/flush), and the set of
// currently-connected subscribers to broadcast committed ops to. Every
// method is safe for concurrent use — callers are separate client
// connections — but internally serializes through one mutex, matching
// ARCHITECTURE.md's "one document, one owner, at any time" doc-actor
// model: a page's ops are applied in one serial order by one process,
// never concurrently.
type Session struct {
	mu          sync.Mutex
	pageID      uuid.UUID
	serverActor string
	page        documentcore.Page
	blocks      map[documentcore.BlockID]*doctext.Text
	clock       oplog.VectorClock
	wal         *wal.Writer
	flush       *flush.Loop
	canApply    CanApplyFunc

	subs      map[uint64]Subscriber
	subActors map[uint64]uuid.UUID // which actor owns each subID, for presence join/leave bookkeeping
	presence  map[uuid.UUID]int    // actor id -> number of currently-open connections (>1 means more than one tab)
	cursors   map[uuid.UUID]CursorEvent // actor id -> their last-known cursor; absent means "not in a block right now"
	nextSubID uint64

	onFlushEnqueueError func(error)
}

// open replays confirmed ops from repo, reconciles any local WAL records
// a crash left un-flushed, and starts a fresh WAL segment + flush loop —
// see the package doc comment and Open's own steps below.
func open(ctx context.Context, pageID uuid.UUID, repo opstore.Repo, walDir string, serverActor string, canApply CanApplyFunc, flushOpts []flush.Option) (*Session, error) {
	if canApply == nil {
		canApply = allowAll
	}

	confirmed, err := repo.ListForPage(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("session: open: loading confirmed ops: %w", err)
	}

	page := documentcore.NewPage(documentcore.PageID(pageID), "")
	blocks := make(map[documentcore.BlockID]*doctext.Text)
	clock := oplog.VectorClock{}
	confirmedIDs := make(map[uuid.UUID]struct{}, len(confirmed))
	for _, l := range confirmed {
		if err := applyReplayedOp(&page, blocks, serverActor, l.Op); err != nil {
			return nil, fmt.Errorf("session: open: replaying confirmed op %s: %w", l.ID, err)
		}
		clock[l.ActorID.String()]++
		confirmedIDs[l.ID] = struct{}{}
	}

	walPath := filepath.Join(walDir, pageID.String()+".wal")
	var toReflush []oplog.LoggedOp
	if _, err := wal.Recover(walPath, func(record []byte) error {
		l, err := oplog.Unmarshal(record)
		if err != nil {
			return fmt.Errorf("decoding WAL record: %w", err)
		}
		if _, already := confirmedIDs[l.ID]; already {
			return nil // already reflected in the confirmed replay above
		}
		if err := applyReplayedOp(&page, blocks, serverActor, l.Op); err != nil {
			return fmt.Errorf("replaying un-flushed WAL op %s: %w", l.ID, err)
		}
		clock[l.ActorID.String()]++
		toReflush = append(toReflush, l)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("session: open: recovering local WAL: %w", err)
	}

	if len(toReflush) > 0 {
		if _, err := repo.AppendBatch(ctx, toReflush); err != nil {
			return nil, fmt.Errorf("session: open: reflushing recovered ops: %w", err)
		}
	}

	// Everything the old WAL segment held is now either already-confirmed
	// or just-reflushed — nothing in it is needed anymore, so it's
	// removed rather than reused. This is this repo's answer to RFC-002
	// §6's "delete once Postgres confirms," at session-open granularity
	// instead of continuous segment rotation (which internal/wal's own
	// doc comment defers as unneeded at this repo's scale).
	if err := os.Remove(walPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("session: open: clearing reconciled WAL: %w", err)
	}
	w, err := wal.OpenWriter(walPath)
	if err != nil {
		return nil, fmt.Errorf("session: open: opening fresh WAL segment: %w", err)
	}

	loop := flush.New(repo, flushOpts...)
	loop.Start(ctx)

	return &Session{
		pageID:              pageID,
		serverActor:         serverActor,
		page:                page,
		blocks:              blocks,
		clock:               clock,
		wal:                 w,
		flush:               loop,
		canApply:            canApply,
		subs:                make(map[uint64]Subscriber),
		subActors:           make(map[uint64]uuid.UUID),
		presence:            make(map[uuid.UUID]int),
		cursors:             make(map[uuid.UUID]CursorEvent),
		onFlushEnqueueError: func(error) {},
	}, nil
}

// applyReplayedOp is the shared step open() runs for every op it replays,
// from either source (confirmed Postgres rows or the un-flushed WAL tail)
// — identical to what ApplyClientOp does to page/blocks, without the
// WAL-append/broadcast/flush-enqueue side effects a replay must not repeat.
func applyReplayedOp(page *documentcore.Page, blocks map[documentcore.BlockID]*doctext.Text, serverActor string, op pageop.Op) error {
	switch op := op.(type) {
	case pageop.Block:
		return applyBlockOp(page, blocks, serverActor, op.Op)
	case pageop.Text:
		text, ok := blocks[op.BlockID]
		if !ok {
			return ErrUnknownBlock
		}
		if _, err := ops.Apply(text, op.Op); err != nil {
			return err
		}
		syncBlockContent(page, op.BlockID, text)
		return nil
	default:
		return fmt.Errorf("session: unknown op type %T", op)
	}
}

// applyBlockOp applies a structural op to page, then keeps blocks (the
// per-block live ropes) consistent with it: InsertBlock seeds a fresh rope
// from the inserted block's initial content, DeleteBlock discards its
// rope, and SetBlockContent reseeds the rope wholesale (a deliberate
// last-write-wins replace — the same semantics the op itself already
// carries, since it can only apply if Prev matches, RFC-002 §2). Every
// other variant (SetBlockKind, SetTitle, MoveBlock) doesn't touch any
// block's text at all.
func applyBlockOp(page *documentcore.Page, blocks map[documentcore.BlockID]*doctext.Text, serverActor string, op documentcore.Op) error {
	if err := page.Apply(op); err != nil {
		return err
	}
	switch op := op.(type) {
	case documentcore.InsertBlock:
		text := doctext.New(serverActor)
		if op.Content.Text != "" {
			if _, err := text.InsertAt(0, op.Content.Text); err != nil {
				return fmt.Errorf("seeding new block's live text: %w", err)
			}
		}
		blocks[op.ID] = text
	case documentcore.DeleteBlock:
		delete(blocks, op.Tombstone.ID)
	case documentcore.SetBlockContent:
		text := doctext.New(serverActor)
		if op.Content.Text != "" {
			if _, err := text.InsertAt(0, op.Content.Text); err != nil {
				return fmt.Errorf("reseeding block's live text: %w", err)
			}
		}
		blocks[op.Block] = text
	}
	return nil
}

// syncBlockContent writes a block's current live text back into the
// page's own Content.Text after a character-granular edit — needed so a
// later SetBlockContent's Prev precondition (documentcore.Page.Apply)
// still matches current state, since Page.Blocks never changes on its own
// as a block's rope is edited.
func syncBlockContent(page *documentcore.Page, blockID documentcore.BlockID, text *doctext.Text) {
	for i := range page.Blocks {
		if page.Blocks[i].ID == blockID {
			page.Blocks[i].Content.Text = text.String()
			return
		}
	}
}

// ApplyClientOp is the whole live-editing pipeline for one client op:
// can_apply, apply (to the page's structure or one block's rope,
// depending on op's tier), durably WAL-sync (the actual ack point —
// RFC-002 §6), broadcast to every other subscriber, then best-effort
// enqueue for the batched Postgres flush. senderSubID excludes the
// submitting connection from the broadcast (its ack is this method's own
// return value, not a Deliver call).
//
// A flush-enqueue failure (the loop is stopped, or its bounded queue is
// full and ctx expires waiting) does not fail the op: the op is already
// durable (WAL) and already visible to every other client (broadcast) by
// that point, matching ARCHITECTURE.md's ack-at-WAL-sync design. It's
// reported via onFlushEnqueueError instead, since the Postgres flush is a
// background durability concern once the client-visible steps are done.
func (s *Session) ApplyClientOp(ctx context.Context, actorID uuid.UUID, actorKind oplog.ActorKind, op pageop.Op, senderSubID uint64) (CommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.canApply(op, actorID, actorKind) {
		return CommitResult{}, ErrDenied
	}

	// The inverse each op tier can compute (documentcore.Op.Invert for a
	// Block op; ops.Apply's own return for a Text op) is what per-actor
	// undo (DATA_MODEL.md's undo_group) would consume — undo isn't wired
	// at the session layer yet (docs/porting/PROGRESS.md), so it's
	// intentionally discarded here rather than half-threaded through an
	// API nothing calls.
	var boundaries *anchor.AnchorRange
	switch o := op.(type) {
	case pageop.Block:
		if err := applyBlockOp(&s.page, s.blocks, s.serverActor, o.Op); err != nil {
			return CommitResult{}, fmt.Errorf("session: apply: %w", err)
		}
	case pageop.Text:
		text, ok := s.blocks[o.BlockID]
		if !ok {
			return CommitResult{}, fmt.Errorf("session: apply: %w", ErrUnknownBlock)
		}
		if _, err := ops.Apply(text, o.Op); err != nil {
			return CommitResult{}, fmt.Errorf("session: apply: %w", err)
		}
		syncBlockContent(&s.page, o.BlockID, text)
		boundaries = text.Boundaries()
	default:
		return CommitResult{}, fmt.Errorf("session: apply: unknown op type %T", op)
	}

	s.clock[actorID.String()]++
	l, err := oplog.New(s.pageID, actorID, actorKind, nil, cloneClock(s.clock), op)
	if err != nil {
		return CommitResult{}, fmt.Errorf("session: apply: building logged op: %w", err)
	}

	record, err := oplog.Marshal(l)
	if err != nil {
		return CommitResult{}, fmt.Errorf("session: apply: marshaling for WAL: %w", err)
	}
	if err := s.wal.Append(record); err != nil {
		return CommitResult{}, fmt.Errorf("session: apply: WAL append: %w", err)
	}

	result := CommitResult{Op: l, Boundaries: boundaries}

	for id, sub := range s.subs {
		if id == senderSubID {
			continue
		}
		sub.Deliver(result)
	}

	if err := s.flush.Enqueue(ctx, l); err != nil {
		s.onFlushEnqueueError(fmt.Errorf("session: enqueue for flush: %w", err))
	}

	return result, nil
}

// Subscribe registers sub, owned by actorID, to receive every future op
// this Session commits (except ones it submits itself via its own subID —
// see ApplyClientOp), every other actor's presence changes, and every
// other actor's cursor moves. The returned id is what the caller passes
// as senderSubID on its own submissions; unsubscribe removes sub and, if
// this was actorID's last open connection, broadcasts that they left and
// clears their cursor (a gone actor's stale caret must not linger for
// everyone still here). present is every distinct actor already here at
// the moment of joining (before actorID's own join is counted); cursors
// is present's own last-known cursor positions (only for those who
// currently have one set) — together, what the caller needs to seed its
// initial "who's here, and where" view without waiting for a future move.
func (s *Session) Subscribe(actorID uuid.UUID, sub Subscriber) (subID uint64, present []uuid.UUID, cursors []CursorEvent, unsubscribe func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	present = make([]uuid.UUID, 0, len(s.presence))
	for a := range s.presence {
		present = append(present, a)
	}
	cursors = make([]CursorEvent, 0, len(s.cursors))
	for _, c := range s.cursors {
		cursors = append(cursors, c)
	}

	id := s.nextSubID
	s.nextSubID++
	s.subs[id] = sub
	s.subActors[id] = actorID

	firstConnection := s.presence[actorID] == 0
	s.presence[actorID]++
	if firstConnection {
		s.broadcastPresenceLocked(PresenceEvent{ActorID: actorID, Joined: true}, id)
	}

	return id, present, cursors, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.subs, id)
		delete(s.subActors, id)
		s.presence[actorID]--
		if s.presence[actorID] <= 0 {
			delete(s.presence, actorID)
			delete(s.cursors, actorID)
			s.broadcastPresenceLocked(PresenceEvent{ActorID: actorID, Joined: false}, id)
			s.broadcastCursorLocked(CursorEvent{ActorID: actorID, BlockID: nil}, id)
		}
	}
}

// SetCursor records actorID's current caret/selection and broadcasts it
// to every other subscriber — fire-and-forget, no ack, and never touches
// the WAL/page/blocks (see CursorEvent's own doc comment). e.BlockID nil
// clears actorID's cursor (they blurred out of every block) rather than
// leaving a stale position parked in a block they've since left.
func (s *Session) SetCursor(actorID uuid.UUID, e CursorEvent, senderSubID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.ActorID = actorID
	if e.BlockID == nil {
		delete(s.cursors, actorID)
	} else {
		s.cursors[actorID] = e
	}
	s.broadcastCursorLocked(e, senderSubID)
}

// broadcastPresenceLocked sends e to every subscriber except exceptSubID
// (the connection whose own join/leave this is — it doesn't need to be
// told about itself). Caller must hold s.mu.
func (s *Session) broadcastPresenceLocked(e PresenceEvent, exceptSubID uint64) {
	for id, sub := range s.subs {
		if id == exceptSubID {
			continue
		}
		sub.DeliverPresence(e)
	}
}

// broadcastCursorLocked mirrors broadcastPresenceLocked, for cursor
// moves. Caller must hold s.mu.
func (s *Session) broadcastCursorLocked(e CursorEvent, exceptSubID uint64) {
	for id, sub := range s.subs {
		if id == exceptSubID {
			continue
		}
		sub.DeliverCursor(e)
	}
}

// Snapshot is the whole page's current live state — title, ordered
// blocks, and each block's live text plus boundary anchors. wsapi's
// connection handler calls this once, for the initial "snapshot" frame,
// so a client connecting to an already non-empty page can render it and
// build valid Text ops immediately instead of only after its own first
// edit (docs/api/collaboration.md).
func (s *Session) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	blocks := make([]BlockSnapshot, len(s.page.Blocks))
	for i, b := range s.page.Blocks {
		text := s.blocks[b.ID]
		blocks[i] = BlockSnapshot{
			ID:         b.ID,
			Kind:       b.Kind,
			Text:       text.String(),
			Marks:      b.Content.Marks,
			Boundaries: text.Boundaries(),
		}
	}
	return Snapshot{PageID: s.page.ID, Title: s.page.Title, Blocks: blocks}
}

// Close stops the flush loop (draining whatever is already buffered, per
// flush.Loop.Stop's own contract) and closes the WAL writer. Ops already
// WAL-synced but not yet flushed remain on disk for the next Open to
// recover.
func (s *Session) Close() error {
	s.flush.Stop()
	return s.wal.Close()
}

func cloneClock(c oplog.VectorClock) oplog.VectorClock {
	out := make(oplog.VectorClock, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}
