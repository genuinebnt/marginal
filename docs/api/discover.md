# Discover API — `DiscoverService`

`docs/ui-mockups/v2/index.html § 09 DISCOVER`, made real: **what is near
this page by MEANING**, as distinct from near by citation
(`graph.md`'s `GraphNeighborhood`) and near in the drawn layout (§ 07's
SPACE lens, `graphalgo.NeighbourMajority`).

Three notions of nearness, deliberately kept as three. The screen exists
because they disagree, and the row worth acting on is always the one
where they do — high cosine, no shared tags, unreachable by link is prose
similarity finding something the graph and the tags both missed, which is
the only reason to run an index at all.

---

## 0. What the vectors actually are

**Not neural embeddings.** There is no model in this repo and shipping
one is out of scope, so `marginal/semantic` builds **hashed,
IDF-weighted term-frequency vectors** — the hashing trick, 256
dimensions, L2-normalised so a dot product *is* the cosine.

That captures **lexical** similarity: two pages using the same uncommon
words score high, and "rope" and "cord" are unrelated to it. The
distinction is stated here, in the proto, in the screen's own inspector,
and in the mockup (whose caption said "384-d embeddings" and was
corrected) because this screen's entire posture is that its figures can
be checked. Swapping in real embeddings later changes `Corpus.Embed` and
nothing else — the index does not know where its vectors came from.

Weighting details worth knowing before changing them:

- **Sublinear TF** (`1 + log tf`), not raw counts. A page that says
  "rope" forty times is about ropes, but it is not ten times more about
  ropes than one saying it four times, and raw counts let long pages
  dominate every neighbourhood.
- **Smoothed IDF** (`log(1 + N/df)`), not the textbook `log(N/df)`. The
  unsmoothed form is exactly 0 for a term in every document — which
  silently deletes that dimension — and undefined for `df = 0`.
- **No stemming.** A stemmer is a per-language table, and getting it
  wrong ("operating" → "oper") merges terms that should not merge. IDF
  already makes both "operation" and "operations" rare and informative.

## 1. `DiscoverService` — the gRPC contract

```protobuf
service DiscoverService {
  rpc Near(NearRequest) returns (NearResponse);
}
```

Full message shapes: `services/document-service/proto/discover.proto`.

**`Near`** returns the `k` pages closest to `source_page_id`, each with
**three separate signals** and never a blended score:

| Signal | Source | Meaning |
|---|---|---|
| `cosine` | `semantic` | lexical similarity, `[0,1]` |
| `shared_tags` / `tag_jaccard` | `docs.page_tags` | the count is what a person reads; the ratio is what compares fairly between a 2-tag page and a 9-tag one |
| `hops` | `graphalgo.BFS` | undirected link distance; **`-1` is unreachable**, deliberately not a large number, because "far" and "not connected at all" are different findings |

A blended score is *unarguable*: when it puts the wrong page first there
is nothing to inspect, because the only thing you can see is the output
of the blend.

**The filter rides the descent.** `topics` and `tags` are applied
*during* the HNSW greedy descent, not to the result set. Post-filtering
asks for `k=5`, throws three away and ships two — and recall collapses
exactly when the filter is narrow, which is when someone is relying on
it. Excluded elements are still traversed (they are the roads) and never
kept.

**Every query runs a brute-force scan beside it.** `recall_at_k` is
measured against that exact answer, not asserted. Speed without recall is
half a result, and an approximate index that never shows its recall is an
index asking to be trusted. On a corpus this size the exact scan is
affordable; when it stops being, this becomes a *sampled* check rather
than a removed one.

`NOT_FOUND` if `source_page_id` does not name a live page.

### Index parameters

`M = 16`, `Mmax0 = 32`, `efConstruction = 64`, `efSearch = 64`,
heuristic neighbour pruning, seed `0x5EED`. Constants rather than
configuration — they are reported on screen precisely so the numbers
beside them can be read against a stated setting, and the seed makes two
builds over one corpus give the same answer, without which the recall
figure cannot be compared with the one printed a minute ago.

The pruning rule is the **heuristic**, not "keep the M closest". Keeping
the closest fills a node's neighbour list from one dense cluster and the
graph loses every long edge — which is what made it navigable.

### Where the index lives (and where it will live)

Rebuilt **per request**, from one table scan. At this repo's scale that
costs less than the screen's latency budget and means the answer is
always over the current document rather than over whatever a background
job last saw. The replacement, when the scan starts to dominate, is not a
cache but a **persisted index**: an `embedding` column written by
`internal/blockproj` as it materialises blocks, with the HNSW graph
stored beside it. That is `v4.4.0` (`RELEASES.md`) and is deliberately
not half-built here.

---

## 2. REST mapping (`api-gateway`)

| Method | Path | RPC |
|---|---|---|
| `GET` | `/discover/{id}?k=&topics=&tags=` | `Near`, page as origin |
| `GET` | `/discover?q=&k=&topics=&tags=` | `Near`, typed text as origin |

**Two origins, one index.** `/discover/{id}` asks "what is near this
page"; `/discover?q=` asks "what is near this sentence", vectorising the
typed string through the same corpus IDF the pages were embedded with —
the same HNSW, the same descent, a query vector that simply did not come
from a row. The text form gets its own path rather than
`/discover/{id}?q=` with a placeholder id, because a typed query has no
page and a URL demanding one would misdescribe the request. When both are
somehow supplied, `q` wins: a client with a focused text box is asking
about its contents.

**A typed query has only one of the three signals**, and the response says
so rather than zeroing the other two. `stats.has_origin` is `false`, and
on every neighbour `shared_tags`/`tag_jaccard` are `0` and `hops` is `-1`
**because the question does not apply** — a sentence carries no tags and
holds no node in the link graph. The screen renders those columns `n/a`
with the reason. Zero would read as "shares none of yours", which is a
finding about the corpus rather than about the query, and the whole
posture of this screen is that its figures can be checked.

A `q` holding nothing the tokenizer keeps (punctuation, or stop words
alone) is **`422 validation_failed`** (this repo's mapping for
`InvalidArgument`, `pages.md` §2), not an empty result: an empty vector
is equidistant from every page, which a client would draw as a ranking.

`topics` and `tags` are comma-separated. An empty parameter produces an
**empty** filter, never a filter naming the empty string — the latter
matches nothing and silently returns no results.

```json
// GET /discover/{id}?k=5
{
  "neighbours": [
    {
      "page_id": "...",
      "title": "Per-actor undo without a stack",
      "excerpt": "A shared undo stack is wrong the moment two people edit one page…",
      "topic_name": "Protocol",
      "topic_color_key": "protocol",
      "tags": ["crdt", "editor", "undo"],
      "cosine": 0.379,
      "shared_tags": 1,
      "tag_jaccard": 0.2,
      "hops": 1
    }
  ],
  "stats": {
    "comparisons": 18,
    "exact_comparisons": 18,
    "hops": 18,
    "layers": 1,
    "recall_at_k": 1.0,
    "candidates": 17,
    "layer_sizes": [18],
    "corpus": 18,
    "top_terms": ["projection", "replay", "invertible", "op"],
    "m": 16,
    "ef_search": 64,
    "dimensions": 256,
    "has_origin": true
  },
  "topics": ["Interface", "Operations", "Protocol", "Research", "Storage"]
}
```

Those are the **real numbers from the seeded corpus**, and they are
unflattering on purpose. Eighteen pages build a **one-layer** tower, so
the descent degenerates to a single layer-0 search that touches every
element: `comparisons == exact_comparisons`, and the index is buying
nothing yet. The screen prints both figures side by side precisely so
that stays visible — a structure has to justify itself at the size it is
actually running at, not at the size its paper was written for. The
layers appear, and the ratio starts to move, somewhere in the low
hundreds of pages.

Every repeated field is `[]`, never `null`, when empty — a client that
only checks `.length` should not also need a null guard.

Auth is a **verified bearer token** (`ADR-013` §1): `Authorization: Bearer
<access token>`, checked by `api-gateway` against `auth-service`'s JWKS, with
the actor id taken from the token's `sub` claim. `X-Actor-Id` is no longer
read anywhere — it was a value the caller wrote and nobody checked.
