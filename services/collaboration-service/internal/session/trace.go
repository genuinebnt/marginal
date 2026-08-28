package session

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/uuid"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/doctext"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/opstore"
	"marginal/collaboration-service/internal/pageop"
)

// TraceStep is one committed op, replayed for real, alongside its own
// already-computed inverse and whether RFC-002 §3's invertibility law —
// apply(invert(op), apply(op, doc)) == doc — actually held for it. This
// is what backs docs/ui-mockups/v2/index.html § 13 TRACE's "real" claim against an
// actual page's actual op log, instead of that mockup's own synthetic,
// fixed nine-op sequence: the same apply/invert code this package already
// runs live (Session.open's replay, Session.Undo/Redo's own gesture
// reversal), never a second, JS-side reimplementation (ADR-012).
type TraceStep struct {
	Op       oplog.LoggedOp
	Inverse  pageop.Op
	LawHolds bool
	// After is the whole document's state once this step's op has been
	// applied — the same Snapshot shape a "snapshot"/"ack" WS frame
	// already carries. Shipped so a client can render "the document at
	// step N" by picking one precomputed entry, never by re-running
	// apply() itself: the algorithm lives in Go, the client only draws
	// what Go already computed (ADR-012's rule, applied here the same way
	// it applies to every other mockup-backing algorithm).
	After Snapshot
}

// MarshalJSON encodes Op/Inverse through oplog.Marshal/pageop.Marshal —
// the same type-tagged wire envelope docs/api/collaboration.md's "ack"/
// "broadcast" frames already use for these exact two types — rather than
// encoding/json's own default interface handling, which would drop the
// "type" tag a client needs to tell one op variant from another
// (oplog.LoggedOp's own doc comment explains why this matters).
func (s TraceStep) MarshalJSON() ([]byte, error) {
	opJSON, err := oplog.Marshal(s.Op)
	if err != nil {
		return nil, err
	}
	invJSON, err := pageop.Marshal(s.Inverse)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Op       json.RawMessage `json:"op"`
		Inverse  json.RawMessage `json:"inverse"`
		LawHolds bool            `json:"law_holds"`
		After    Snapshot        `json:"after"`
	}{Op: opJSON, Inverse: invJSON, LawHolds: s.LawHolds, After: s.After})
}

// Trace replays every confirmed op for pageID from empty state and, for
// each one, verifies the invertibility law for real. A step is checked by
// replaying the log twice — once through it, once stopping short — rather
// than deep-cloning live rope state mid-replay: no Clone exists on
// documentcore.Page or doctext.Text today, and building one just for a
// debug view is exactly the speculative machinery this repo's speed rules
// say to skip. Replaying twice is O(n²) over the whole log, an accepted
// cost for a view invoked once per request at this repo's demo scale, not
// a per-keystroke path. Read-only: touches no live Session, no WAL, no
// broadcast — safe to call for a page that also has a live session open.
func Trace(ctx context.Context, pageID uuid.UUID, repo opstore.Repo, serverActor string) ([]TraceStep, error) {
	confirmed, err := repo.ListForPage(ctx, pageID)
	if err != nil {
		return nil, fmt.Errorf("session: trace: listing ops: %w", err)
	}

	page := documentcore.NewPage(documentcore.PageID(pageID), "")
	blocks := make(map[documentcore.BlockID]*doctext.Text)
	steps := make([]TraceStep, len(confirmed))
	for i, l := range confirmed {
		inverse, err := applyReplayedOp(&page, blocks, serverActor, l.Op)
		if err != nil {
			return nil, fmt.Errorf("session: trace: replaying op %d (%s): %w", i, l.ID, err)
		}
		steps[i] = TraceStep{
			Op:       l,
			Inverse:  inverse,
			LawHolds: checkInvertibilityLaw(confirmed[:i], confirmed[:i+1], inverse, serverActor),
			After:    buildSnapshot(page, blocks),
		}
	}
	return steps, nil
}

// checkInvertibilityLaw is RFC-002 §3's law, checked rather than assumed:
// replaying prefix (every op strictly before the step under test) must
// produce the same observable document as replaying upToAndIncluding
// (prefix plus that one step) and then applying its own inverse.
func checkInvertibilityLaw(prefix, upToAndIncluding []oplog.LoggedOp, inverse pageop.Op, serverActor string) bool {
	before, beforeBlocks, err := replayOps(prefix, serverActor)
	if err != nil {
		return false
	}
	afterPage, afterBlocks, err := replayOps(upToAndIncluding, serverActor)
	if err != nil {
		return false
	}
	if _, err := applyReplayedOp(&afterPage, afterBlocks, serverActor, inverse); err != nil {
		return false
	}
	return reflect.DeepEqual(snapshotReplayState(before, beforeBlocks), snapshotReplayState(afterPage, afterBlocks))
}

// replayOps replays ops from empty state — the same applyReplayedOp
// Session.open's own replay loop uses, exposed standalone (no live
// Session, no WAL, no broadcast) so checkInvertibilityLaw can run it
// twice per step without disturbing Trace's own ongoing forward replay.
func replayOps(ops []oplog.LoggedOp, serverActor string) (documentcore.Page, map[documentcore.BlockID]*doctext.Text, error) {
	page := documentcore.NewPage(documentcore.PageID(uuid.Nil), "")
	blocks := make(map[documentcore.BlockID]*doctext.Text)
	for _, l := range ops {
		if _, err := applyReplayedOp(&page, blocks, serverActor, l.Op); err != nil {
			return documentcore.Page{}, nil, err
		}
	}
	return page, blocks, nil
}

// replayState is a normalized, comparable snapshot of a replayed
// page+blocks: Title plus, per block in document order, its identity,
// containment, kind, live text, and marks. Two states with an equal
// replayState count as the same document for the law's purposes even if
// their underlying doctext ropes carry different tombstone bookkeeping
// (e.g. "never touched" vs. "deleted then reinserted back") — the law is
// about observable document content, exactly what the mockup's own
// JSON.stringify(doc) comparison checked, not internal CRDT metadata.
type replayState struct {
	Title  string
	Blocks []replayBlock
}

type replayBlock struct {
	ID     documentcore.BlockID
	Parent *documentcore.BlockID
	Kind   documentcore.BlockKind
	Text   string
	Marks  []documentcore.Mark
}

func snapshotReplayState(page documentcore.Page, blocks map[documentcore.BlockID]*doctext.Text) replayState {
	out := replayState{Title: page.Title, Blocks: make([]replayBlock, len(page.Blocks))}
	for i, b := range page.Blocks {
		text := ""
		if t := blocks[b.ID]; t != nil {
			text = t.String()
		}
		out.Blocks[i] = replayBlock{ID: b.ID, Parent: b.Parent, Kind: b.Kind, Text: text, Marks: b.Content.Marks}
	}
	return out
}
