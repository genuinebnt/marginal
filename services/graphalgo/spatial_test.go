package graphalgo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"marginal/graphalgo"
)

func TestNeighbourMajorityCountsTheNodeItself(t *testing.T) {
	// One node, no spatial neighbours at all: it is its own majority, not
	// an absence. A page alone in a corner is still that topic.
	got := graphalgo.NeighbourMajority(nil, map[graphalgo.NodeID]string{"a": "protocol"})
	assert.Equal(t, map[graphalgo.NodeID]string{"a": "protocol"}, got)
}

func TestNeighbourMajorityDisagreesWithTheDeclaredLabel(t *testing.T) {
	// The case the SPACE lens exists for: `a` says protocol, but it is
	// surrounded by storage. 1 vote for protocol (its own), 3 for storage.
	adjacent := []graphalgo.DelaunayPair{{A: "a", B: "b"}, {A: "a", B: "c"}, {A: "a", B: "d"}}
	label := map[graphalgo.NodeID]string{
		"a": "protocol", "b": "storage", "c": "storage", "d": "storage",
	}
	got := graphalgo.NeighbourMajority(adjacent, label)
	assert.Equal(t, "storage", got["a"], "a sits inside storage territory even though it declares protocol")
	assert.Equal(t, "storage", got["b"])
}

func TestNeighbourMajorityBreaksATieTowardsTheDeclaredLabel(t *testing.T) {
	// One vote each way — storage (its own), protocol, research. A
	// neighbourhood this evenly split says nothing about `a`, so overruling
	// what `a` declares would manufacture a disagreement out of no evidence.
	// This is the hardest case here: it is the one where the "obvious"
	// answer (alphabetical) is wrong for a reason that is about the product,
	// not about the sort.
	adjacent := []graphalgo.DelaunayPair{{A: "a", B: "b"}, {A: "a", B: "c"}}
	label := map[graphalgo.NodeID]string{"a": "storage", "b": "protocol", "c": "research"}
	first := graphalgo.NeighbourMajority(adjacent, label)
	assert.Equal(t, "storage", first["a"], "an evenly split neighbourhood must not overrule the page")

	// And whatever it answers, it must answer the same thing every run.
	for i := 0; i < 20; i++ {
		assert.Equal(t, first, graphalgo.NeighbourMajority(adjacent, label))
	}
}

func TestNeighbourMajorityBreaksAnUntopicedTieAlphabetically(t *testing.T) {
	// No own label to prefer, so determinism is all that is left. Pinned
	// because "whichever map iteration order gave us" is not a rule.
	adjacent := []graphalgo.DelaunayPair{{A: "a", B: "b"}, {A: "a", B: "c"}}
	label := map[graphalgo.NodeID]string{"a": "", "b": "storage", "c": "protocol"}
	for i := 0; i < 20; i++ {
		assert.Equal(t, "protocol", graphalgo.NeighbourMajority(adjacent, label)["a"])
	}
}

func TestNeighbourMajoritySkipsNodesWithNoEvidence(t *testing.T) {
	// Untopiced, and every spatial neighbour untopiced too. Absent from the
	// result rather than mapped to "" — the caller draws "no topic" in its
	// own hue, and inventing a majority here would overwrite a real state.
	adjacent := []graphalgo.DelaunayPair{{A: "a", B: "b"}}
	label := map[graphalgo.NodeID]string{"a": "", "b": ""}
	got := graphalgo.NeighbourMajority(adjacent, label)
	assert.NotContains(t, got, graphalgo.NodeID("a"))
}

func TestNeighbourMajorityInfersALabelForAnUntopicedNode(t *testing.T) {
	// Untopiced itself, but sitting among protocol pages. It has evidence,
	// so SPACE can colour it — which is the lens's most useful case: it
	// suggests what an untopiced page is probably about.
	adjacent := []graphalgo.DelaunayPair{{A: "a", B: "b"}, {A: "a", B: "c"}}
	label := map[graphalgo.NodeID]string{"a": "", "b": "protocol", "c": "protocol"}
	got := graphalgo.NeighbourMajority(adjacent, label)
	assert.Equal(t, "protocol", got["a"])
}
