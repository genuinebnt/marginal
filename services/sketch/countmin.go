package sketch

import "sort"

// CountMin answers "how many times did I see THIS one", in fixed memory,
// with a one-sided error: it may overestimate and can never underestimate.
//
// THE IDEA. d independent hash functions, each into a row of w counters. Every
// increment bumps one counter per row. To query, take the MINIMUM across the
// rows — a counter can only be inflated by collisions, so the smallest of d
// independently-collided estimates is the closest to the truth.
//
// THE ONE-SIDEDNESS IS THE POINT, and it is what makes the structure safe for
// the question § 12 asks it ("which pages are heavy"): an overestimate might
// put a page on the list that does not belong, which you notice; an
// underestimate would silently omit a page that does, which you do not. A
// structure whose error can go either way is unusable for a top-k.
type CountMin struct {
	depth, width int
	table        [][]uint32
	total        uint64
}

// NewCountMin builds a d×w table. Both are clamped to at least 1; the useful
// range is d in 3..8 (more rows, exponentially less chance every row collides
// at once) and w in the hundreds (wider rows, fewer collisions per row).
func NewCountMin(depth, width int) *CountMin {
	if depth < 1 {
		depth = 1
	}
	if width < 1 {
		width = 1
	}
	t := make([][]uint32, depth)
	for i := range t {
		t[i] = make([]uint32, width)
	}
	return &CountMin{depth: depth, width: width, table: t}
}

// Add increments one counter per row.
func (c *CountMin) Add(s string, n uint32) {
	c.total += uint64(n)
	for row := 0; row < c.depth; row++ {
		c.table[row][c.index(s, row)] += n
	}
}

// Estimate is the minimum across rows — never below the true count.
func (c *CountMin) Estimate(s string) uint32 {
	best := ^uint32(0)
	for row := 0; row < c.depth; row++ {
		if v := c.table[row][c.index(s, row)]; v < best {
			best = v
		}
	}
	return best
}

func (c *CountMin) index(s string, row int) int {
	return int(seeded(s, uint64(row)+1) % uint64(c.width))
}

// Total is every increment ever applied. Exact, because it is a counter and
// not a sketch — worth having beside the estimates as the one number in the
// panel that cannot be wrong.
func (c *CountMin) Total() uint64 { return c.total }

// Bytes is the table's own size: 4 bytes per counter, fixed.
func (c *CountMin) Bytes() int { return c.depth * c.width * 4 }

// Dimensions returns (depth, width) — § 12 prints "4 × 24 table" from it.
func (c *CountMin) Dimensions() (int, int) { return c.depth, c.width }

// TopK ranks candidates by their ESTIMATE.
//
// Candidates are passed in rather than discovered, because a Count-Min sketch
// genuinely cannot enumerate its own keys — it never stored one. That is not
// a limitation of this implementation; it is the same fact as the privacy
// property, seen from the query side. A real top-k pairs the sketch with a
// small heap of candidate keys, which is what the caller is doing here.
//
// Ties break on the key, so the list is stable across runs.
func (c *CountMin) TopK(candidates []string, k int) []Counted {
	out := make([]Counted, 0, len(candidates))
	for _, s := range candidates {
		out = append(out, Counted{Key: s, Estimate: c.Estimate(s)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Estimate != out[j].Estimate {
			return out[i].Estimate > out[j].Estimate
		}
		return out[i].Key < out[j].Key
	})
	if k > 0 && len(out) > k {
		out = out[:k]
	}
	return out
}

// Counted is one key with its sketched count.
type Counted struct {
	Key      string `json:"key"`
	Estimate uint32 `json:"estimate"`
	// Exact is filled by the caller that has the real counts — § 12 shows
	// them side by side, which is the only way "overestimate only, never
	// under" is a claim rather than an assertion.
	Exact uint32 `json:"exact"`
}
