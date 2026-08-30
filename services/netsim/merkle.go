package netsim

import (
	"encoding/binary"
	"fmt"
)

// MerkleNode is one node of § 14's TREE · MERKLE COMPARISON.
//
// The leaves are fixed-size chunks of the text, not blocks: this
// module simulates the character tier, and a Merkle tree over one
// block's characters is the tier the transform actually runs on.
// The real service hashes the block tree itself.
type MerkleNode struct {
	ID    string `json:"id"`
	Depth int    `json:"depth"`
	// Hash is this subtree's digest on the LEFT replica.
	Hash string `json:"hash"`
	// OtherHash is the same subtree on the right replica. When the two
	// differ, this subtree contains a divergence somewhere.
	OtherHash string `json:"other_hash"`
	Equal     bool   `json:"equal"`
	// Divergence marks the FIRST unequal node on a root-to-leaf path —
	// the highest node that already knows something is wrong. It is
	// the answer the whole tree exists to give: comparing two
	// documents costs one hash when they agree, and log n when they
	// do not.
	Divergence bool  `json:"divergence"`
	Children   []int `json:"children,omitempty"`
	// Sample is the leaf's own text, for the tooltip. Empty on
	// interior nodes.
	Sample string `json:"sample,omitempty"`
}

// MerkleView is the drawn tree plus what it concluded.
type MerkleView struct {
	Nodes []MerkleNode `json:"nodes"`
	// Equal is the root comparison — one hash, the whole document.
	Equal bool `json:"equal"`
	// ComparedNodes is how many nodes a real reconciliation would have
	// had to fetch to find the divergence. This is the number the
	// structure earns: it is not len(Nodes).
	ComparedNodes int `json:"compared_nodes"`
	LeafBytes     int `json:"leaf_bytes"`
}

const merkleLeafBytes = 8

// fnv1a, folded to 32 bits and printed short — this is a display
// digest, not a security one, and calling it a hash without saying
// which kind would be the kind of vagueness this repo avoids.
func digest(parts ...string) string {
	var h uint64 = 14695981039346656037
	for _, p := range parts {
		for i := 0; i < len(p); i++ {
			h ^= uint64(p[i])
			h *= 1099511628211
		}
		h ^= 0xff
		h *= 1099511628211
	}
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], h)
	return fmt.Sprintf("%02x%02x", b[0], b[1])
}

func chunks(s string) []string {
	if s == "" {
		return []string{""}
	}
	var out []string
	for i := 0; i < len(s); i += merkleLeafBytes {
		out = append(out, s[i:min(i+merkleLeafBytes, len(s))])
	}
	return out
}

// CompareMerkle builds one tree over both texts and marks where they
// first disagree.
//
// Both replicas are chunked to the SAME leaf count (the longer one
// wins, the shorter is padded with empty leaves) so the trees are
// structurally comparable. Comparing two differently-shaped trees
// would report every node as unequal and locate nothing — which is
// the failure mode this drawing is meant to make obvious.
func CompareMerkle(left, right string) MerkleView {
	a, b := chunks(left), chunks(right)
	for len(a) < len(b) {
		a = append(a, "")
	}
	for len(b) < len(a) {
		b = append(b, "")
	}

	view := MerkleView{LeafBytes: merkleLeafBytes}
	var build func(lo, hi, depth int) int
	build = func(lo, hi, depth int) int {
		idx := len(view.Nodes)
		view.Nodes = append(view.Nodes, MerkleNode{Depth: depth})
		if hi-lo == 1 {
			n := &view.Nodes[idx]
			n.ID = fmt.Sprintf("c%d", lo)
			n.Hash, n.OtherHash = digest(a[lo]), digest(b[lo])
			n.Equal = n.Hash == n.OtherHash
			n.Sample = a[lo]
			return idx
		}
		mid := (lo + hi) / 2
		l, r := build(lo, mid, depth+1), build(mid, hi, depth+1)
		n := &view.Nodes[idx]
		n.ID = fmt.Sprintf("n%d-%d", lo, hi)
		n.Hash = digest(view.Nodes[l].Hash, view.Nodes[r].Hash)
		n.OtherHash = digest(view.Nodes[l].OtherHash, view.Nodes[r].OtherHash)
		n.Equal = n.Hash == n.OtherHash
		n.Children = []int{l, r}
		return idx
	}
	root := build(0, len(a), 0)
	view.Equal = view.Nodes[root].Equal

	// Walk down from the root, counting what a reconciliation would
	// actually fetch, and mark the first unequal node on each path.
	var walk func(i int, parentUnequal bool)
	walk = func(i int, parentUnequal bool) {
		n := &view.Nodes[i]
		view.ComparedNodes++
		if n.Equal {
			return // one hash settled this whole subtree
		}
		if !parentUnequal {
			n.Divergence = true
		}
		for _, c := range n.Children {
			walk(c, true)
		}
	}
	walk(root, false)
	return view
}
