// Package semantic turns a page's prose into a vector and answers "what is
// near this" over those vectors — the algorithm behind § 09 DISCOVER.
//
// WHAT THE EMBEDDING IS, STATED PLAINLY. These are not neural embeddings.
// There is no model in this repo and no intention of shipping one at this
// scope, so the vectors here are hashed, IDF-weighted term frequencies —
// the "hashing trick", a real and old vector-space representation of text
// with well-understood properties. It captures LEXICAL similarity: two pages
// that use the same uncommon words score high. It does not capture meaning,
// so "rope" and "cord" are unrelated to it.
//
// That distinction is drawn here rather than hidden because § 09's whole
// argument rests on the third signal disagreeing with the other two — a page
// with high cosine, no shared tags and no link path is the reason to run an
// index at all. That argument survives a lexical vector; what does not
// survive is calling it something it is not. The screen says "hashed TF-IDF"
// and the mockup's caption is corrected to match. Swapping in real
// embeddings later changes Embed and nothing else: the index below is
// agnostic about where its vectors came from.
package semantic

import (
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"unicode"
)

// Dim is the vector width. 256 is far more than the vocabulary of a
// notebook-sized corpus needs, which keeps hash collisions rare enough that
// two unrelated terms almost never share a dimension — the one real failure
// mode of the hashing trick.
const Dim = 256

// Vector is one page's position in term space, always L2-normalised so that
// a dot product IS the cosine.
type Vector [Dim]float32

// Document is one page's text as the index sees it: an id and its terms.
type Document struct {
	ID    string
	Terms []string
}

// Tokenize lowercases, splits on anything that is not a letter or a digit,
// and drops stop words and 1-character tokens.
//
// Stemming is deliberately absent. A stemmer is a per-language table, and
// getting it wrong ("operating" -> "oper") merges terms that should not
// merge — worse for a small technical corpus than leaving "operation" and
// "operations" as two dimensions, where the IDF weighting below already
// makes both rare and both informative.
func Tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 2 || stopWords[f] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// bucket maps a term to a dimension. FNV-1a because it is stable across
// processes and across runs — an index whose dimensions move between two
// builds is an index whose stored vectors are meaningless.
func bucket(term string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(term))
	return int(h.Sum32() % Dim)
}

// Corpus holds the document frequencies IDF needs. IDF is a property of the
// COLLECTION, not of a document, so a vector cannot be computed for one page
// in isolation — which is why Embed hangs off this rather than being a free
// function.
type Corpus struct {
	docFreq map[string]int
	docs    int
}

// NewCorpus counts, for each term, how many documents contain it at all
// (not how many times) — that is what makes IDF a measure of rarity rather
// than of verbosity.
func NewCorpus(docs []Document) *Corpus {
	c := &Corpus{docFreq: map[string]int{}, docs: len(docs)}
	for _, d := range docs {
		seen := map[string]bool{}
		for _, t := range d.Terms {
			if seen[t] {
				continue
			}
			seen[t] = true
			c.docFreq[t]++
		}
	}
	return c
}

// Embed projects one document into the term space.
//
// Sublinear TF (1 + log tf) rather than raw counts: a page that says "rope"
// forty times is about ropes, but it is not ten times more about ropes than
// one that says it four times, and raw counts make long pages dominate every
// neighbourhood.
//
// Smoothed IDF (log(1 + N/df)) rather than the textbook log(N/df): the
// unsmoothed form is exactly 0 for a term present in every document, which
// silently deletes that dimension; and it is undefined for df = 0, which a
// query term outside the corpus can be.
func (c *Corpus) Embed(terms []string) Vector {
	tf := map[string]int{}
	for _, t := range terms {
		tf[t]++
	}
	var v Vector
	for term, n := range tf {
		idf := math.Log(1 + float64(c.docs)/float64(1+c.docFreq[term]))
		w := (1 + math.Log(float64(n))) * idf
		v[bucket(term)] += float32(w)
	}
	normalize(&v)
	return v
}

// normalize scales to unit length. A zero vector (a page with no indexable
// terms at all) stays zero rather than becoming NaN — it then has cosine 0
// against everything, which is the honest answer for a page with no prose.
func normalize(v *Vector) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return
	}
	inv := float32(1 / math.Sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
}

// Cosine is the dot product, which for unit vectors IS the cosine. Kept as a
// named function anyway: the caller should never have to know that the
// vectors happen to be normalised for the arithmetic to be right.
func Cosine(a, b Vector) float64 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return float64(sum)
}

// TopTerms are the highest-weighted terms in a document, for explaining WHY
// two pages scored close. A similarity number nobody can interrogate is a
// similarity number nobody should trust.
func (c *Corpus) TopTerms(terms []string, n int) []string {
	tf := map[string]int{}
	for _, t := range terms {
		tf[t]++
	}
	type scored struct {
		term string
		w    float64
	}
	all := make([]scored, 0, len(tf))
	for term, count := range tf {
		idf := math.Log(1 + float64(c.docs)/float64(1+c.docFreq[term]))
		all = append(all, scored{term, (1 + math.Log(float64(count))) * idf})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].w != all[j].w {
			return all[i].w > all[j].w
		}
		return all[i].term < all[j].term
	})
	if len(all) > n {
		all = all[:n]
	}
	out := make([]string, len(all))
	for i, s := range all {
		out[i] = s.term
	}
	return out
}

// stopWords is short on purpose. A long list is a language-specific asset
// that has to be maintained; IDF already suppresses anything that appears
// everywhere. These are here only because they are so frequent that they
// waste dimensions before IDF gets a chance to weight them down.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "that": true, "this": true,
	"with": true, "not": true, "but": true, "are": true, "was": true,
	"you": true, "its": true, "it": true, "is": true, "in": true, "of": true,
	"to": true, "on": true, "as": true, "at": true, "by": true, "or": true,
	"an": true, "be": true, "from": true, "which": true, "what": true,
	"has": true, "have": true, "can": true, "will": true, "would": true,
	"one": true, "two": true, "than": true, "then": true, "when": true,
	"there": true, "their": true, "they": true, "them": true, "into": true,
	"more": true, "most": true, "some": true, "any": true, "all": true,
	"every": true, "each": true, "other": true, "same": true, "does": true,
	"do": true, "did": true, "how": true, "why": true, "who": true,
}
