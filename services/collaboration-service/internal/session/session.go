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
	"log/slog"
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

// maxUndoDepth bounds how many gestures (single ops or groups) each
// actor's undo stack keeps — the same eviction reasoning as
// documentcore.History's own maxDepth: an unbounded stack holding onto
// every inverse an actor ever produced this session is a slow memory leak
// (Content strings, Block tombstones), not a correctness issue.
const maxUndoDepth = 100

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

// ErrOutOfRange is RestoreTo's own target-step validation failure — see
// its doc comment for the valid range.
var ErrOutOfRange = errors.New("session: restore target step out of range")

// CanApplyFunc is RFC-002 §5's one auditable authorization chokepoint.
// Every op passes through it before touching the page or any block's rope.
//
// pageID is a parameter rather than something the implementation is
// expected to already know, because authorization is per-PAGE: a role is
// held in a space, a page belongs to a space, and one Manager serves every
// page in the process. Without it the chokepoint cannot express the
// question it exists to answer (ADR-013 §3).
type CanApplyFunc func(pageID uuid.UUID, op pageop.Op, actorID uuid.UUID, actorKind oplog.ActorKind) bool

func allowAll(uuid.UUID, pageop.Op, uuid.UUID, oplog.ActorKind) bool { return true }

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
	Parent     *documentcore.BlockID  `json:"parent"`
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
// connections — and every one of them takes s.mu for its whole body, so
// ARCHITECTURE.md's "one document, one owner, at any time" (a page's ops
// are applied in one serial order, never concurrently) holds in the
// sense that matters: no two ops interleave. The mechanism is a shared
// mutex, though, not an owning actor/goroutine with exclusive access —
// worth being precise about for a future Rust port specifically, since
// the two map to genuinely different designs (Arc<Mutex<Session>> vs. an
// actor task reached only through channels, no lock anywhere). Decide
// which one the port targets before translating this type; don't infer
// it from this comment's earlier, looser wording.
// undoGroup is one undo-able gesture — RFC-002 §3's "undo pops the newest
// undo_group belonging to this actor, never the newest op." inverses holds
// every op the gesture contained, already inverted, in the order the
// *original* ops were applied (oldest first) — undoing the gesture means
// walking this slice back-to-front (the last-applied original op must be
// undone first); redoing it means walking a freshly-built group (of the
// undo's own resulting inverses) front-to-back. groupID mirrors the
// client-supplied undo_group this gesture shared, purely for logging/
// debugging — nil is a valid group of one, same as the wire field itself.
type undoGroup struct {
	groupID  *uuid.UUID
	inverses []pageop.Op
}

type Session struct {
	mu          sync.Mutex
	pageID      uuid.UUID
	serverActor string
	// repo backs RestoreTo's own call to Trace — the same confirmed-log
	// replay a standalone GET /collab/pages/{id}/trace request uses,
	// invoked here instead against the live session's own actor/lock so
	// the inverses it computes can be applied straight through
	// commitOpLocked. Not used for anything else Session already does
	// (open() reads confirmed ops directly via its own repo parameter).
	repo     opstore.Repo
	page     documentcore.Page
	blocks   map[documentcore.BlockID]*doctext.Text
	clock    oplog.VectorClock
	wal      *wal.Writer
	flush    *flush.Loop
	canApply CanApplyFunc

	// undo is reconstructed from collab.ops on every session open (RFC-002
	// §3: "putting it in the log rather than a client-side stack") — it
	// survives a reconnect. redo is not: it's purely in-memory, always
	// starts empty on open, and is cleared for an actor the moment any new
	// op of theirs commits (recordUndoLocked), the same "a new op
	// invalidates redo" rule documentcore.History already enforces.
	undo map[uuid.UUID][]undoGroup
	redo map[uuid.UUID][]undoGroup

	subs      map[uint64]Subscriber
	subActors map[uint64]uuid.UUID      // which actor owns each subID, for presence join/leave bookkeeping
	presence  map[uuid.UUID]int         // actor id -> number of currently-open connections (>1 means more than one tab)
	cursors   map[uuid.UUID]CursorEvent // actor id -> their last-known cursor; absent means "not in a block right now"
	nextSubID uint64

	// blockIndex mirrors page.Blocks' ordering (id -> its current slice
	// index) so a character-op's syncBlockContent — the actual hot path,
	// once per keystroke — doesn't have to linear-scan page.Blocks to find
	// which one to update. Rebuilt wholesale on any structural (block-scope)
	// op instead of maintained incrementally: those are comparatively rare
	// (insert/delete/kind-change/move), so an O(n) rebuild there is the
	// right place to pay for O(1) lookups on the much hotter text-op path.
	blockIndex map[documentcore.BlockID]int

	logger *slog.Logger
}

// rebuildBlockIndex must be called (with s.mu held) after any op that
// changes page.Blocks' membership or order.
func (s *Session) rebuildBlockIndex() {
	s.blockIndex = make(map[documentcore.BlockID]int, len(s.page.Blocks))
	for i, b := range s.page.Blocks {
		s.blockIndex[b.ID] = i
	}
}

// open replays confirmed ops from repo, reconciles any local WAL records
// a crash left un-flushed, and starts a fresh WAL segment + flush loop —
// see the package doc comment and Open's own steps below.
func open(ctx context.Context, pageID uuid.UUID, repo opstore.Repo, walDir string, serverActor string, canApply CanApplyFunc, flushOpts []flush.Option, logger *slog.Logger) (*Session, error) {
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
	undoStacks := make(map[uuid.UUID][]undoGroup)
	confirmedIDs := make(map[uuid.UUID]struct{}, len(confirmed))
	for _, l := range confirmed {
		inverse, err := applyReplayedOp(&page, blocks, serverActor, l.Op)
		if err != nil {
			return nil, fmt.Errorf("session: open: replaying confirmed op %s: %w", l.ID, err)
		}
		appendUndoGroup(undoStacks, l.ActorID, l.UndoGroup, inverse)
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
		inverse, err := applyReplayedOp(&page, blocks, serverActor, l.Op)
		if err != nil {
			return fmt.Errorf("replaying un-flushed WAL op %s: %w", l.ID, err)
		}
		appendUndoGroup(undoStacks, l.ActorID, l.UndoGroup, inverse)
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
	// Deliberately NOT ctx: ctx here is the request that happened to
	// trigger this page's first open (an HTTP handler's context in
	// wsapi), not this Session's own lifetime — Manager holds every
	// Session open indefinitely (this package's own doc comment), so
	// binding the flush loop to that first caller's context meant the
	// loop silently died the moment that specific client disconnected,
	// while the Session itself lived on and kept accepting edits that
	// then never reached Postgres. The loop's actual lifetime is
	// Session.Close (which calls flush.Loop.Stop) — background.Context
	// here just means "not cancelled by anything else."
	loop.Start(context.Background())

	if logger == nil {
		logger = slog.Default()
	}

	s := &Session{
		pageID:      pageID,
		serverActor: serverActor,
		repo:        repo,
		page:        page,
		blocks:      blocks,
		clock:       clock,
		wal:         w,
		flush:       loop,
		canApply:    canApply,
		subs:        make(map[uint64]Subscriber),
		subActors:   make(map[uint64]uuid.UUID),
		presence:    make(map[uuid.UUID]int),
		cursors:     make(map[uuid.UUID]CursorEvent),
		undo:        undoStacks,
		redo:        make(map[uuid.UUID][]undoGroup),
		logger:      logger,
	}
	s.rebuildBlockIndex()
	return s, nil
}

// applyReplayedOp is the shared step open() runs for every op it replays,
// from either source (confirmed Postgres rows or the un-flushed WAL tail)
// — identical to what ApplyClientOp does to page/blocks, without the
// WAL-append/broadcast/flush-enqueue side effects a replay must not repeat.
// Returns op's own inverse, computed the same way ApplyClientOp computes
// it live, so open() can rebuild every actor's undo stack from the log
// exactly as it happened the first time (RFC-002 §3).
func applyReplayedOp(page *documentcore.Page, blocks map[documentcore.BlockID]*doctext.Text, serverActor string, op pageop.Op) (pageop.Op, error) {
	switch op := op.(type) {
	case pageop.Block:
		if err := applyBlockOp(page, blocks, serverActor, op.Op); err != nil {
			return nil, err
		}
		return pageop.Block{Op: op.Op.Invert()}, nil
	case pageop.Text:
		text, ok := blocks[op.BlockID]
		if !ok {
			return nil, ErrUnknownBlock
		}
		inverse, err := ops.Apply(text, op.Op)
		if err != nil {
			return nil, err
		}
		syncBlockContent(page, op.BlockID, text)
		return pageop.Text{BlockID: op.BlockID, Op: inverse}, nil
	default:
		return nil, fmt.Errorf("session: unknown op type %T", op)
	}
}

// appendUndoGroup pushes inverseOp onto actorID's undo stack in stacks,
// merging it into the current top group when groupID is non-nil and
// matches that group's own id (another op from the same gesture) —
// otherwise starting a fresh group. A nil groupID always starts a new
// group ("undo_group IS NULL means a group of one," RFC-002 §3). Evicts
// the oldest group once an actor's stack exceeds maxUndoDepth via
// copy+reslice, the same technique and reasoning as
// documentcore.History.Record's own eviction.
func appendUndoGroup(stacks map[uuid.UUID][]undoGroup, actorID uuid.UUID, groupID *uuid.UUID, inverseOp pageop.Op) {
	groups := stacks[actorID]
	if n := len(groups); n > 0 && groupID != nil && groups[n-1].groupID != nil && *groups[n-1].groupID == *groupID {
		groups[n-1].inverses = append(groups[n-1].inverses, inverseOp)
	} else {
		groups = append(groups, undoGroup{groupID: groupID, inverses: []pageop.Op{inverseOp}})
	}
	if len(groups) > maxUndoDepth {
		copy(groups, groups[1:])
		groups = groups[:maxUndoDepth]
	}
	stacks[actorID] = groups
}

// applyBlockOp keeps blocks (the per-block live ropes) consistent with a
// structural op: InsertBlock seeds a fresh rope from the inserted block's
// initial content, DeleteBlock discards its rope, and SetBlockContent
// reseeds the rope wholesale (a deliberate last-write-wins replace — the
// same semantics the op itself already carries, since it can only apply
// if Prev matches, RFC-002 §2). Every other variant (SetBlockKind,
// SetTitle, MoveBlock) doesn't touch any block's text at all.
//
// Any new rope is built BEFORE page.Apply, not after: an earlier version
// applied to page first and seeded blocks second, so a seeding failure
// (InsertAt rejecting the initial content) returned an error while
// leaving page.Blocks and blocks disagreeing about whether the block
// existed at all — Session.Snapshot indexes by page.Blocks and
// unconditionally dereferences the matching *doctext.Text, so that gap
// was a nil-pointer panic waiting for the next client to connect, not
// just a returned error the caller could recover from.
func applyBlockOp(page *documentcore.Page, blocks map[documentcore.BlockID]*doctext.Text, serverActor string, op documentcore.Op) error {
	var seeded *doctext.Text
	switch op := op.(type) {
	case documentcore.InsertBlock:
		text := doctext.New(serverActor)
		if op.Content.Text != "" {
			if _, err := text.InsertAt(0, op.Content.Text); err != nil {
				return fmt.Errorf("seeding new block's live text: %w", err)
			}
		}
		seeded = text
	case documentcore.SetBlockContent:
		text := doctext.New(serverActor)
		if op.Content.Text != "" {
			if _, err := text.InsertAt(0, op.Content.Text); err != nil {
				return fmt.Errorf("reseeding block's live text: %w", err)
			}
		}
		seeded = text
	}

	if err := page.Apply(op); err != nil {
		return err
	}

	switch op := op.(type) {
	case documentcore.InsertBlock:
		blocks[op.ID] = seeded
	case documentcore.DeleteBlock:
		delete(blocks, op.Tombstone.ID)
	case documentcore.SetBlockContent:
		blocks[op.Block] = seeded
	}
	return nil
}

// syncBlockContent writes a block's current live text back into the
// page's own Content.Text after a character-granular edit — needed so a
// later SetBlockContent's Prev precondition (documentcore.Page.Apply)
// still matches current state, since Page.Blocks never changes on its own
// as a block's rope is edited. A linear scan — fine for applyReplayedOp's
// one-time-at-startup use (open() replaying confirmed/WAL ops before any
// Session, and therefore any blockIndex, exists yet); ApplyClientOp's own
// per-keystroke hot path uses Session.syncBlockContentFast instead.
func syncBlockContent(page *documentcore.Page, blockID documentcore.BlockID, text *doctext.Text) {
	for i := range page.Blocks {
		if page.Blocks[i].ID == blockID {
			page.Blocks[i].Content.Text = text.String()
			return
		}
	}
}

// syncBlockContentFast is syncBlockContent via s.blockIndex — O(1) instead
// of a scan over every block, on the path a keystroke actually takes.
func (s *Session) syncBlockContentFast(blockID documentcore.BlockID, text *doctext.Text) {
	if i, ok := s.blockIndex[blockID]; ok {
		s.page.Blocks[i].Content.Text = text.String()
	}
}

// ApplyClientOp is the whole live-editing pipeline for one client op:
// can_apply, apply (to the page's structure or one block's rope,
// depending on op's tier), durably WAL-sync (the actual ack point —
// RFC-002 §6), broadcast to every other subscriber, then best-effort
// enqueue for the batched Postgres flush. senderSubID excludes the
// submitting connection from the broadcast (its ack is this method's own
// return value, not a Deliver call). undoGroupID is the client-supplied
// grouping id (docs/api/collaboration.md §2) — nil for a group of one.
//
// A flush-enqueue failure (the loop is stopped, or its bounded queue is
// full and ctx expires waiting) does not fail the op: the op is already
// durable (WAL) and already visible to every other client (broadcast) by
// that point, matching ARCHITECTURE.md's ack-at-WAL-sync design. It's
// logged instead, since the Postgres flush is a background durability
// concern once the client-visible steps are done.
func (s *Session) ApplyClientOp(ctx context.Context, actorID uuid.UUID, actorKind oplog.ActorKind, op pageop.Op, undoGroupID *uuid.UUID, senderSubID uint64) (CommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.canApply(s.pageID, op, actorID, actorKind) {
		return CommitResult{}, ErrDenied
	}

	result, inverse, err := s.commitOpLocked(ctx, actorID, actorKind, op, undoGroupID, senderSubID)
	if err != nil {
		return CommitResult{}, fmt.Errorf("session: apply: %w", err)
	}

	// A genuine client edit always lands on undo and always invalidates
	// whatever redo was pointing at — the same rule documentcore.History
	// already enforces, generalized to per-actor groups.
	appendUndoGroup(s.undo, actorID, undoGroupID, inverse)
	delete(s.redo, actorID)

	return result, nil
}

// applyPageOpLocked mutates the session's live state (page structure or
// one block's rope, depending on op's tier) and returns that op's own
// inverse — documentcore.Op.Invert() for a Block op, ops.Apply's own
// return for a Text op — plus, only for a Text op, that block's current
// boundary anchors. Caller must hold s.mu.
func (s *Session) applyPageOpLocked(op pageop.Op) (*anchor.AnchorRange, pageop.Op, error) {
	switch o := op.(type) {
	case pageop.Block:
		if err := applyBlockOp(&s.page, s.blocks, s.serverActor, o.Op); err != nil {
			return nil, nil, err
		}
		s.rebuildBlockIndex()
		return nil, pageop.Block{Op: o.Op.Invert()}, nil
	case pageop.Text:
		text, ok := s.blocks[o.BlockID]
		if !ok {
			return nil, nil, ErrUnknownBlock
		}
		inverse, err := ops.Apply(text, o.Op)
		if err != nil {
			return nil, nil, err
		}
		s.syncBlockContentFast(o.BlockID, text)
		return text.Boundaries(), pageop.Text{BlockID: o.BlockID, Op: inverse}, nil
	default:
		return nil, nil, fmt.Errorf("unknown op type %T", op)
	}
}

// commitOpLocked is the pipeline ApplyClientOp, Undo, and Redo all share
// once an op has been authorized and is ready to become the next entry in
// this page's log: apply it, stamp a LoggedOp (tagged with undoGroupID),
// WAL-append, broadcast to every subscriber but senderSubID, best-effort
// flush-enqueue. Returns the CommitResult for the caller's own ack and
// op's own inverse — which stack (undo or redo, and whether it clears the
// other) that inverse belongs on is the caller's decision, not this
// method's, since ApplyClientOp and Undo/Redo make that decision
// differently. Caller must hold s.mu and must already have checked
// can_apply.
func (s *Session) commitOpLocked(ctx context.Context, actorID uuid.UUID, actorKind oplog.ActorKind, op pageop.Op, undoGroupID *uuid.UUID, senderSubID uint64) (CommitResult, pageop.Op, error) {
	boundaries, inverse, err := s.applyPageOpLocked(op)
	if err != nil {
		return CommitResult{}, nil, err
	}

	s.clock[actorID.String()]++
	l, err := oplog.New(s.pageID, actorID, actorKind, undoGroupID, cloneClock(s.clock), op)
	if err != nil {
		return CommitResult{}, nil, fmt.Errorf("building logged op: %w", err)
	}

	record, err := oplog.Marshal(l)
	if err != nil {
		return CommitResult{}, nil, fmt.Errorf("marshaling for WAL: %w", err)
	}
	if err := s.wal.Append(record); err != nil {
		return CommitResult{}, nil, fmt.Errorf("WAL append: %w", err)
	}

	result := CommitResult{Op: l, Boundaries: boundaries}

	for id, sub := range s.subs {
		if id == senderSubID {
			continue
		}
		sub.Deliver(result)
	}

	if err := s.flush.Enqueue(ctx, l); err != nil {
		s.logger.Error("session: enqueue for flush failed", "page_id", s.pageID, "op_id", l.ID, "err", err)
	}

	return result, inverse, nil
}

// Undo pops actorID's own most recent undo_group (or single op) and
// re-applies its inverse(s) against current state, oldest-original-op
// undone last — docs/api/collaboration.md §2.1 has the full contract,
// including what "not atomic across a multi-op group" means when op k of
// a group fails partway. An empty undo stack is a no-op: (nil, nil), not
// an error, the same contract documentcore.History.Undo already holds
// itself to. Each op undone is itself logged, WAL'd, and broadcast exactly
// like an ordinary client op — from every other connection's point of
// view, an undo is indistinguishable from actorID submitting N ops,
// because it is one.
func (s *Session) Undo(ctx context.Context, actorID uuid.UUID, actorKind oplog.ActorKind, senderSubID uint64) ([]CommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	groups := s.undo[actorID]
	if len(groups) == 0 {
		return nil, nil
	}
	group := groups[len(groups)-1]
	remaining := groups[:len(groups)-1]

	redoGroupID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("session: undo: %w", err)
	}

	var results []CommitResult
	var produced []pageop.Op // filled newest-original-op-first; reversed below
	for i := len(group.inverses) - 1; i >= 0; i-- {
		op := group.inverses[i]
		if !s.canApply(s.pageID, op, actorID, actorKind) {
			s.undo[actorID] = append(remaining, undoGroup{groupID: group.groupID, inverses: group.inverses[:i+1]})
			return results, ErrDenied
		}
		result, redoOp, err := s.commitOpLocked(ctx, actorID, actorKind, op, &redoGroupID, senderSubID)
		if err != nil {
			s.undo[actorID] = append(remaining, undoGroup{groupID: group.groupID, inverses: group.inverses[:i+1]})
			return results, fmt.Errorf("session: undo: %w", err)
		}
		results = append(results, result)
		produced = append(produced, redoOp)
	}

	s.undo[actorID] = remaining
	for i := len(produced) - 1; i >= 0; i-- {
		appendUndoGroup(s.redo, actorID, &redoGroupID, produced[i])
	}
	return results, nil
}

// Redo pops actorID's own most recent redo group (only ever populated by
// that actor's own prior Undo — see the Session.undo/redo field comment
// for why redo isn't reconstructed from collab.ops) and re-applies it
// oldest-first, restoring the gesture Undo just reverted. Same no-op and
// non-atomic-across-a-group contract as Undo.
func (s *Session) Redo(ctx context.Context, actorID uuid.UUID, actorKind oplog.ActorKind, senderSubID uint64) ([]CommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	groups := s.redo[actorID]
	if len(groups) == 0 {
		return nil, nil
	}
	group := groups[len(groups)-1]
	remaining := groups[:len(groups)-1]

	undoGroupID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("session: redo: %w", err)
	}

	var results []CommitResult
	for i := 0; i < len(group.inverses); i++ {
		op := group.inverses[i]
		if !s.canApply(s.pageID, op, actorID, actorKind) {
			s.redo[actorID] = append(remaining, undoGroup{groupID: group.groupID, inverses: group.inverses[i:]})
			return results, ErrDenied
		}
		result, undoOp, err := s.commitOpLocked(ctx, actorID, actorKind, op, &undoGroupID, senderSubID)
		if err != nil {
			s.redo[actorID] = append(remaining, undoGroup{groupID: group.groupID, inverses: group.inverses[i:]})
			return results, fmt.Errorf("session: redo: %w", err)
		}
		results = append(results, result)
		appendUndoGroup(s.undo, actorID, &undoGroupID, undoOp)
	}

	s.redo[actorID] = remaining
	return results, nil
}

// RestoreTo brings the live document back to its state as of right after
// step toStep of this page's confirmed op log (0-indexed, the same
// indexing GET /collab/pages/{id}/trace's own "steps" array uses) —
// docs/ui-mockups/v2/index.html § 17 HISTORY's "restore to a point," made real.
//
// This is repeated undo, not a restore-from-backup: RFC-002 §3's whole
// argument for why undo applies an inverse against current state rather
// than swapping in a stored snapshot applies here identically, just
// walking further back in one call than Undo's own single-gesture scope.
// Trace already computes, for every confirmed op, the inverse that
// exactly reverses it against the state that existed right after it
// applied — so restoring is: replay to find those inverses (Trace's own
// job, unchanged), then apply the inverses for every step after toStep,
// most recent first, through the normal commitOpLocked pipeline (WAL,
// broadcast, flush-enqueue) — from every other connection's point of
// view indistinguishable from actorID submitting that many ordinary ops,
// because it is that.
//
// Shares Trace's own documented visibility boundary: it replays
// confirmed rows from repo, so a WAL tail not yet flushed at the moment
// RestoreTo runs is invisible to it — the same gap Trace already states
// plainly rather than hides, and one a deliberate, manual "restore"
// click is very unlikely to race in practice (flush is sub-second).
// Holding s.mu for the whole call (same as Undo/Redo) is what actually
// matters for correctness here: no other op can commit against this
// session while the restore is being computed and applied, so the
// confirmed-log snapshot Trace reads at the start of the call cannot go
// stale mid-restore.
//
// The whole restore becomes one new undo group for actorID — undoing it
// re-applies the original ops it just reverted, the same symmetry Undo
// and Redo already give each other.
func (s *Session) RestoreTo(ctx context.Context, actorID uuid.UUID, actorKind oplog.ActorKind, toStep int, senderSubID uint64) ([]CommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	steps, err := Trace(ctx, s.pageID, s.repo, s.serverActor)
	if err != nil {
		return nil, fmt.Errorf("session: restore: %w", err)
	}
	if toStep < 0 || toStep >= len(steps) {
		return nil, ErrOutOfRange
	}
	if toStep == len(steps)-1 {
		return nil, nil // already there — Undo/Redo's own "empty stack" no-op contract
	}

	groupID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("session: restore: %w", err)
	}

	var results []CommitResult
	// produced collects each reverted step's own original op (commitOpLocked's
	// "redoOp", here really "the op that undoes this restore"), built
	// newest-step-first as steps are undone in that same order below.
	// Unlike Undo's own "produced" (which targets s.redo, consumed
	// ascending by Redo and so needs reversing first), this one is pushed
	// onto s.undo as-is: Undo consumes descending, and undoing this
	// restore must itself reapply steps[toStep+1..] in ascending original
	// order (each one's own precondition — e.g. SetBlockContent's prev —
	// was captured against the step right before it, not against
	// whatever state a wrong order would present it with) — which is
	// exactly what a descending-stored, descending-consumed group gives.
	var produced []pageop.Op
	for i := len(steps) - 1; i > toStep; i-- {
		op := steps[i].Inverse
		if !s.canApply(s.pageID, op, actorID, actorKind) {
			return results, ErrDenied
		}
		result, redoOp, err := s.commitOpLocked(ctx, actorID, actorKind, op, &groupID, senderSubID)
		if err != nil {
			return results, fmt.Errorf("session: restore: %w", err)
		}
		results = append(results, result)
		produced = append(produced, redoOp)
	}

	for _, op := range produced {
		appendUndoGroup(s.undo, actorID, &groupID, op)
	}
	return results, nil
}

// Subscription is the handle Subscribe returns: an id (what the caller
// passes as senderSubID on its own submissions, so ApplyClientOp/SetCursor
// can exclude the connection that sent them from its own broadcast), the
// state needed to seed a joining client's initial view, and a Close
// method to unsubscribe. A real handle type instead of Subscribe's
// earlier four positional returns (one of them a closure capturing the
// Session, id, and actorID) — the same shape a Rust port would want as a
// guard type, not four loose values a caller has to keep straight.
type Subscription struct {
	ID uint64
	// Present is every distinct actor already here at the moment of
	// joining (before actorID's own join is counted).
	Present []uuid.UUID
	// Cursors is Present's own last-known cursor positions (only for
	// those who currently have one set).
	Cursors []CursorEvent
	// Snapshot is the page's own state as of the exact same instant
	// Present/Cursors were read — taken under the same lock acquisition
	// as the rest of Subscribe, not a second, separate Session.Snapshot()
	// call. Calling Snapshot() separately after Subscribe returned had a
	// real race: another actor's op could commit and broadcast in the
	// gap between the two lock acquisitions, since the new subscriber was
	// already registered to receive it — the broadcast landed AND the
	// following Snapshot() already reflected it, so the client applied
	// that one op twice.
	Snapshot Snapshot

	session *Session
	actorID uuid.UUID
}

// Close unsubscribes and, if this was actorID's last open connection,
// broadcasts that they left and clears their cursor (a gone actor's stale
// caret must not linger for everyone still here).
func (sub *Subscription) Close() {
	s := sub.session
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, sub.ID)
	delete(s.subActors, sub.ID)
	s.presence[sub.actorID]--
	if s.presence[sub.actorID] <= 0 {
		delete(s.presence, sub.actorID)
		delete(s.cursors, sub.actorID)
		s.broadcastPresenceLocked(PresenceEvent{ActorID: sub.actorID, Joined: false}, sub.ID)
		s.broadcastCursorLocked(CursorEvent{ActorID: sub.actorID, BlockID: nil}, sub.ID)
	}
}

// Subscribe registers sub, owned by actorID, to receive every future op
// this Session commits (except ones it submits itself via its own subID —
// see ApplyClientOp), every other actor's presence changes, and every
// other actor's cursor moves.
func (s *Session) Subscribe(actorID uuid.UUID, sub Subscriber) *Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()

	present := make([]uuid.UUID, 0, len(s.presence))
	for a := range s.presence {
		present = append(present, a)
	}
	cursors := make([]CursorEvent, 0, len(s.cursors))
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

	return &Subscription{
		ID:       id,
		Present:  present,
		Cursors:  cursors,
		Snapshot: s.snapshotLocked(),
		session:  s,
		actorID:  actorID,
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
// blocks, and each block's live text plus boundary anchors. Use
// Subscribe's own Snapshot field when connecting for the first time (the
// same state, taken atomically with registering as a subscriber); this
// method is for anything that needs a fresh read later, mid-connection.
func (s *Session) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked()
}

// snapshotLocked is Snapshot's body, callable by anything that already
// holds s.mu (Subscribe, so its own Snapshot field reflects the exact
// same instant as Present/Cursors, not a second, separately-locked read
// that could race a commit landing in between).
func (s *Session) snapshotLocked() Snapshot {
	return buildSnapshot(s.page, s.blocks)
}

// buildSnapshot is snapshotLocked's body, as a free function over any
// page/blocks pair rather than a live Session's own fields — so Trace can
// build the same shape at every replay step without a Session to call it
// through (its own doc comment has the "why free function" reasoning).
func buildSnapshot(page documentcore.Page, blocks map[documentcore.BlockID]*doctext.Text) Snapshot {
	out := make([]BlockSnapshot, len(page.Blocks))
	for i, b := range page.Blocks {
		// text == nil should be unreachable now that applyBlockOp seeds a
		// block's rope before, not after, admitting it into page.Blocks —
		// kept as a defence-in-depth guard against that invariant
		// (spanning two separate data structures) breaking again, rather
		// than a nil-pointer panic taking down whichever connection asked
		// for a snapshot.
		text := blocks[b.ID]
		if text == nil {
			out[i] = BlockSnapshot{ID: b.ID, Parent: b.Parent, Kind: b.Kind, Marks: b.Content.Marks}
			continue
		}
		out[i] = BlockSnapshot{
			ID:         b.ID,
			Parent:     b.Parent,
			Kind:       b.Kind,
			Text:       text.String(),
			Marks:      b.Content.Marks,
			Boundaries: text.Boundaries(),
		}
	}
	return Snapshot{PageID: page.ID, Title: page.Title, Blocks: out}
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
