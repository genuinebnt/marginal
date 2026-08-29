// Package sketch is the three probabilistic structures § 12 ANALYTICS is
// built on: HyperLogLog for "how many distinct", Count-Min for "how many of
// each", and a t-digest for "what does the distribution look like".
//
// THE ARGUMENT THE SCREEN MAKES, and the reason these are sketches rather
// than counters: no per-person row is stored, so there is nothing to leak and
// nothing to subpoena. The sketches ARE the privacy mechanism, not an
// optimisation that happens to have a privacy benefit.
//
// Every estimate here is displayed beside its exact answer and its error,
// which is why this package computes both. A sketch that hides its error is
// indistinguishable from a wrong number, and the whole point of putting these
// on a screen is that they can be caught being wrong.
package sketch

// hash64 is FNV-1a.
//
// Not a cryptographic hash and not trying to be — what these structures need
// is avalanche (one flipped input bit changes about half the output bits) and
// determinism across processes, and FNV-1a gives both in six lines. A
// stronger hash would cost more per element for accuracy nobody can measure
// at this scale, and a weaker one (a sum, a length) would correlate the
// register index with the leading-zero count and quietly destroy HLL's
// estimate.
//
// Determinism matters more than it looks: the same stream must produce the
// same estimate in the browser and on the server, or the screen's "exact vs
// estimate" comparison would be measuring the hash rather than the sketch.
func hash64(s string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}

// mix is a 64-bit finalizer (splitmix64's). FNV-1a's low bits are weak on
// short, similar strings — "page-1" and "page-2" differ in one byte — and
// Count-Min derives several independent-looking hashes by seeding, which
// amplifies exactly that weakness into correlated rows. One mixing step
// costs three multiplies and removes the problem.
func mix(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

func seeded(s string, seed uint64) uint64 { return mix(hash64(s) ^ (seed * 0x9e3779b97f4a7c15)) }
