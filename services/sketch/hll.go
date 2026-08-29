package sketch

import (
	"math"
	"math/bits"
)

// HLL is a HyperLogLog: "how many DISTINCT things did I see", in constant
// memory, without keeping any of them.
//
// THE IDEA. Hash each element to 64 bits. Use the first p bits to pick one of
// m = 2^p registers, and record in that register the position of the leading
// 1 in the REST of the hash. A run of k leading zeros turns up about once
// every 2^k elements, so the longest run any register has seen is a (very
// noisy) estimate of how many distinct elements landed there. Averaging m of
// those noisy estimates harmonically, with a bias constant, is the whole
// algorithm — and it is why the memory is m bytes regardless of whether you
// fed it a thousand elements or a billion.
//
// The privacy property falls out rather than being added: a register holds a
// small integer, and there is no sequence of registers from which an element
// can be recovered. Nothing here is anonymised — nothing was ever stored.
type HLL struct {
	p         uint8
	registers []uint8
}

// NewHLL builds a sketch with 2^p registers. p is clamped to [4,16]: below 4
// the correction constants are undefined, and above 16 the memory stops being
// the point of using one.
func NewHLL(p uint8) *HLL {
	if p < 4 {
		p = 4
	}
	if p > 16 {
		p = 16
	}
	return &HLL{p: p, registers: make([]uint8, 1<<p)}
}

// Add records one element. Adding the SAME element again cannot change any
// register — it hashes to the same index and the same rank, and the register
// already holds at least that — which is exactly the property that makes the
// estimate a count of distinct things. It is also the most legible thing to
// demonstrate on § 12: paste a line twice and watch the number not move.
func (h *HLL) Add(s string) {
	x := mix(hash64(s))
	idx := x >> (64 - h.p)
	// The remaining bits, with the index bits shifted out. The `| 1<<(p-1)`
	// guard bounds the run length so an all-zero remainder cannot report a
	// rank larger than the bits actually available.
	rest := (x << h.p) | (1 << (h.p - 1))
	rank := uint8(bits.LeadingZeros64(rest)) + 1
	if rank > h.registers[idx] {
		h.registers[idx] = rank
	}
}

// Estimate is the distinct count.
//
// Two regimes, and the small one is not an optimisation — the raw harmonic
// estimator is badly biased when most registers are still empty, which at a
// notebook's scale is the common case. Linear counting over the empty-register
// fraction is exact enough there, and the switch at 2.5m is Flajolet's own.
func (h *HLL) Estimate() float64 {
	m := float64(len(h.registers))
	sum, zeros := 0.0, 0
	for _, r := range h.registers {
		sum += 1.0 / float64(uint64(1)<<r)
		if r == 0 {
			zeros++
		}
	}
	raw := alpha(len(h.registers)) * m * m / sum
	if raw <= 2.5*m && zeros > 0 {
		return m * math.Log(m/float64(zeros))
	}
	return raw
}

// Merge unions another sketch into this one — the register-wise max, because
// "the longest run either of us saw" is the longest run the union saw.
//
// This is the property that makes HLL usable across a distributed system at
// all: two nodes can count independently and the counts combine exactly, with
// no coordination and no double-counting of elements both saw. Nothing else
// in this package has that.
func (h *HLL) Merge(other *HLL) {
	if other == nil || other.p != h.p {
		return
	}
	for i, r := range other.registers {
		if r > h.registers[i] {
			h.registers[i] = r
		}
	}
}

// Registers exposes the raw registers — § 12 draws them as a bar chart,
// because the shape is the argument: a roughly even skyline is a healthy
// hash, and a few tall spikes over empty ground is a hash that is not
// spreading.
func (h *HLL) Registers() []uint8 { return h.registers }

// Bytes is the sketch's own size. One byte per register, and it does not
// grow — which is the number § 12 puts in its top bar beside the event count.
func (h *HLL) Bytes() int { return len(h.registers) }

// alpha is the bias-correction constant. The three special cases are from the
// HLL paper; the general form is the limit for large m.
func alpha(m int) float64 {
	switch m {
	case 16:
		return 0.673
	case 32:
		return 0.697
	case 64:
		return 0.709
	default:
		return 0.7213 / (1 + 1.079/float64(m))
	}
}

// StandardError is the expected relative error, 1.04/sqrt(m). Reported so the
// screen can say whether the error it is SHOWING is within what the structure
// promised — an error inside the bound is the sketch working, and one outside
// it is a finding.
func (h *HLL) StandardError() float64 {
	return 1.04 / math.Sqrt(float64(len(h.registers)))
}
