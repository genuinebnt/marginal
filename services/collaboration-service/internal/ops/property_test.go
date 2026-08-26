package ops

import (
	"testing"

	"pgregory.net/rapid"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/doctext"
)

// TestPropertyApplyInverseRoundTrips drives a doctext.Text through a random
// sequence of InsertText/DeleteText ops, applying each op's returned inverse
// immediately afterward, and requires the text to be exactly as it was
// before that op — RFC-002 §3's invertibility law, at this package's
// granularity. Anchors are drawn from ids the text has actually seen, the
// same differential-testing shape as anchor's own property test.
func TestPropertyApplyInverseRoundTrips(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := doctext.New("actor-1")
		var ids []anchor.ItemID

		steps := rapid.IntRange(1, 30).Draw(t, "steps")
		for i := 0; i < steps; i++ {
			before := text.String()

			var op Op
			isInsert := len(ids) == 0 || rapid.Bool().Draw(t, "insert")
			if isInsert {
				s := rapid.StringOfN(rapid.RuneFrom([]rune("abcXYZ é")), 1, 6, -1).Draw(t, "text")
				var at *anchor.Anchor
				if len(ids) > 0 && rapid.Bool().Draw(t, "anchored") {
					id := ids[rapid.IntRange(0, len(ids)-1).Draw(t, "anchorIdx")]
					bias := anchor.Before
					if rapid.Bool().Draw(t, "after") {
						bias = anchor.After
					}
					a := anchor.Anchor{Item: id, Bias: bias}
					at = &a
				}
				op = InsertText{At: at, Text: s}
			} else {
				aIdx := rapid.IntRange(0, len(ids)-1).Draw(t, "startIdx")
				bIdx := rapid.IntRange(0, len(ids)-1).Draw(t, "endIdx")
				if aIdx > bIdx {
					aIdx, bIdx = bIdx, aIdx
				}
				op = DeleteText{
					Range: anchor.AnchorRange{
						Start: anchor.Anchor{Item: ids[aIdx], Bias: anchor.Before},
						End:   anchor.Anchor{Item: ids[bIdx], Bias: anchor.After},
					},
				}
			}

			inverse, err := Apply(text, op)
			if err != nil {
				t.Fatalf("Apply(%#v) failed: %v", op, err)
			}

			if del, ok := inverse.(DeleteText); ok {
				// op was an InsertText; the ids it created are exactly the
				// range this DeleteText names — actor-1's IDGenerator hands
				// out consecutive counters regardless of insert position,
				// so every counter between Start and End was assigned to
				// this insert.
				for c := del.Range.Start.Item.Counter; c <= del.Range.End.Item.Counter; c++ {
					ids = append(ids, anchor.ItemID{Actor: "actor-1", Counter: c})
				}
			}

			if _, err := Apply(text, inverse); err != nil {
				t.Fatalf("Apply(inverse %#v) failed: %v", inverse, err)
			}
			if text.String() != before {
				t.Fatalf("apply(inverse, apply(op, text)) != text: op=%#v before=%q after=%q", op, before, text.String())
			}
		}
	})
}
