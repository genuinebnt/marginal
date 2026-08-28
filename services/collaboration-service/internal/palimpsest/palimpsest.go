// Package palimpsest builds one block's whole character history as a
// real persistent, tombstoned array — docs/ui-mockups/history.html's own
// central claim, made real: "the whole edit history is a list of ops
// applied to a tombstoned char array: a delete sets a version stamp, it
// never removes. Reading version v is the filter `ins <= v < del`, so
// every version is addressable from ONE structure — which is what
// 'persistent data structure' means, and why history costs storage
// rather than time."
//
// Neither doctext.Text nor anchor.Log already gives this for free: both
// exist to answer "what does this block look like RIGHT NOW," and
// anchor.Log's own tombstoning only keeps enough to resolve an Anchor —
// identity and liveness, never the character's own rune or who deleted
// it (session.go's own applyReplayedOp/Trace have the same shape: they
// replay forward to compute *current* state plus one inverse per step,
// never a full historical array). This package is a second, parallel
// replay over the exact same confirmed ops (blockproj's own precedent
// for "a projection, not a second writer" — RFC-002's op log is still
// the only source of truth), scoped to one block, purely to answer "what
// did every character in this block ever look like, and when did it
// live."
package palimpsest

import (
	"fmt"

	"github.com/google/uuid"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/doctext"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/pageop"
)

// Char is one character this block's live text has ever held, kept
// forever once inserted. InsertStep/DeleteStep index into the same
// confirmed op log GET /collab/pages/{id}/trace's own "steps" array
// uses, so a client can drive one scrubber against both. DeleteStep is
// nil for a character still live right now.
type Char struct {
	Rune        rune       `json:"rune"`
	InsertStep  int        `json:"insert_step"`
	InsertActor uuid.UUID  `json:"insert_actor"`
	DeleteStep  *int       `json:"delete_step,omitempty"`
	DeleteActor *uuid.UUID `json:"delete_actor,omitempty"`
}

// Build replays confirmed's character-tier (pageop.Text) ops scoped to
// blockID and returns the block's whole tombstoned character array,
// oldest-inserted first — chars' own slice index is permanent once
// assigned; nothing is ever removed or reordered, only marked. Any other
// op in confirmed (a pageop.Block op, or a pageop.Text op naming a
// different block) is skipped, not an error — a real page interleaves
// edits across many blocks and other blocks' own structural changes.
//
// serverActor must be the exact same tag the live session's own
// doctext.Text used for this block (session.go's applyBlockOp calls
// doctext.New(serverActor) on every InsertBlock) — anchor.ItemID's
// identity is {actor tag, counter} (RFC-002's own "ItemID is about
// ordering insert operations, not authorship," anchor.go), so a
// mismatched tag here would assign THIS replay's own fresh ids instead
// of recognizing the ones later ops in confirmed already name, and every
// DeleteText after the first InsertText would fail to resolve.
func Build(confirmed []oplog.LoggedOp, blockID documentcore.BlockID, serverActor string) ([]Char, error) {
	text := doctext.New(serverActor)
	var chars []Char
	// live[k] is the chars index behind the k-th currently-live rune —
	// the same "identity vs. current position" split anchor.Log itself
	// keeps internally, duplicated here only because anchor.Log doesn't
	// expose the rune value or the deleting actor.
	var live []int

	for step, l := range confirmed {
		pt, ok := l.Op.(pageop.Text)
		if !ok || pt.BlockID != blockID {
			continue
		}

		switch o := pt.Op.(type) {
		case ops.InsertText:
			pos, err := resolveInsertPosition(text, o.At)
			if err != nil {
				return nil, fmt.Errorf("palimpsest: step %d: %w", step, err)
			}
			runes := []rune(o.Text)
			if len(runes) == 0 {
				continue
			}
			if _, err := text.InsertAt(pos, o.Text); err != nil {
				return nil, fmt.Errorf("palimpsest: step %d: %w", step, err)
			}
			newIdx := make([]int, len(runes))
			for i, r := range runes {
				newIdx[i] = len(chars)
				chars = append(chars, Char{Rune: r, InsertStep: step, InsertActor: l.ActorID})
			}
			grown := make([]int, 0, len(live)+len(newIdx))
			grown = append(grown, live[:pos]...)
			grown = append(grown, newIdx...)
			grown = append(grown, live[pos:]...)
			live = grown

		case ops.DeleteText:
			start, end, err := resolveRange(text, o.Range)
			if err != nil {
				return nil, fmt.Errorf("palimpsest: step %d: %w", step, err)
			}
			if start == end {
				continue
			}
			if err := text.DeleteRange(start, end); err != nil {
				return nil, fmt.Errorf("palimpsest: step %d: %w", step, err)
			}
			deleteStep, deleteActor := step, l.ActorID
			for _, idx := range live[start:end] {
				chars[idx].DeleteStep = &deleteStep
				chars[idx].DeleteActor = &deleteActor
			}
			live = append(live[:start], live[end:]...)

		case ops.NoOp:
			// Nothing landed (an empty insert, or a start==end delete
			// already handled above by the resolved-range check) — no
			// character history to record.

		default:
			return nil, fmt.Errorf("palimpsest: step %d: unknown ops.Op %T", step, o)
		}
	}
	return chars, nil
}

// resolveInsertPosition/resolveRange mirror internal/ops' own unexported
// helpers of the same name exactly (anchor.Anchor -> live rune offset,
// via doctext.Text.Resolve) — small enough (both call straight through
// to already-tested anchor/doctext code) that a second, local copy here
// is the honest cost of not exporting internal implementation details
// out of a package whose only other consumer is itself.
func resolveInsertPosition(text *doctext.Text, at *anchor.Anchor) (int, error) {
	if at == nil {
		return 0, nil
	}
	resolved := text.Resolve(*at)
	if resolved.Kind == anchor.Unknown {
		return 0, ops.ErrUnknownAnchor
	}
	return resolved.Offset, nil
}

func resolveRange(text *doctext.Text, r anchor.AnchorRange) (start, end int, err error) {
	startResolved := text.Resolve(r.Start)
	if startResolved.Kind == anchor.Unknown {
		return 0, 0, ops.ErrUnknownAnchor
	}
	endResolved := text.Resolve(r.End)
	if endResolved.Kind == anchor.Unknown {
		return 0, 0, ops.ErrUnknownAnchor
	}
	start, end = startResolved.Offset, endResolved.Offset
	if start > end {
		start, end = end, start
	}
	return start, end, nil
}

// AtVersion filters chars down to what was live right after step v —
// history.html's own "reading version v is the filter ins <= v < del."
func AtVersion(chars []Char, v int) []Char {
	var out []Char
	for _, c := range chars {
		if c.InsertStep <= v && (c.DeleteStep == nil || v < *c.DeleteStep) {
			out = append(out, c)
		}
	}
	return out
}

// LiveText reads AtVersion's v == "now" case back into a plain string —
// used by this package's own tests to check the persistent array agrees
// with what the ordinary doctext.Text replay (Trace, Session.open) says
// the block's live text actually is.
func LiveText(chars []Char) string {
	runes := make([]rune, 0, len(chars))
	for _, c := range chars {
		if c.DeleteStep == nil {
			runes = append(runes, c.Rune)
		}
	}
	return string(runes)
}
