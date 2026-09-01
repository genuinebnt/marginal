# API — Search

**Status:** Implemented in Go (`services/document-service/internal/search`,
`internal/bktree`) — `Search`, `SuggestTitles`, both computed fresh (or
freshly-indexed) per request/refresh, never cached client-side. Backs
`docs/ui-mockups/v2/index.html § 06 SEARCH` (`v2.5.0`, `docs/planning/RELEASES.md`) —
real full-text search plus real fuzzy title matching, never a second
implementation in the browser (`ADR-012`).
**Owners:** `document-service` (gRPC `SearchService`) · `api-gateway` (REST translation)
**Related:** `internal/bktree`'s own doc comment (the metric-tree
algorithm) · `docs/architecture/DATA_MODEL.md` §2 (`docs.pages`/
`docs.blocks`' own `search_vector` columns, migration `00004`) ·
`docs/api/pages.md` (`ListBacklinks` — the reverse-link-index half of
"search is full text plus the link graph, not vectors")

Same two-contract shape as `pages.md`/`graph.md`:

```
   browser  ──REST/JSON──▶  api-gateway  ──gRPC/protobuf──▶  document-service
            §2 below                      §1 below
```

`SearchService` lives on `document-service`'s own deployable (`ADR-001`):
it's a query surface over tables `document-service` already owns
(`docs.pages`, `docs.blocks`), not a separate store with its own scaling
profile, state, or failure mode — the same reasoning `GraphService`
already sets a precedent for.

---

## 1. `SearchService` — the gRPC contract

```protobuf
service SearchService {
  rpc Search(SearchRequest) returns (SearchResponse);
  rpc SuggestTitles(SuggestTitlesRequest) returns (SuggestTitlesResponse);
}
```

Full message shapes: `services/document-service/proto/search.proto`.

**`Search`** — real full-text search, Postgres FTS standing in for
Tantivy at this repo's scope (`docs/planning/RELEASES.md`'s own wording:
"an in-process, embeddable-index choice, not a new service"). Runs
`websearch_to_tsquery` (handles a plain query box's quoted phrases and
bare-word OR/AND the way a user actually types, unlike `plainto_tsquery`'s
AND-only or `to_tsquery`'s operator syntax) against two `GENERATED
ALWAYS ... STORED` `tsvector` columns — `docs.pages.search_vector`
(title) and `docs.blocks.search_vector` (block text) — each backed by
its own GIN index (migration `00004_search_vectors.sql`). A hit is
either title-only (`block_id`/`snippet` absent) or a block match
(`snippet` is `ts_headline`'s own `<b>...</b>`-wrapped output — the
actual reason the hit matched, not re-derived client-side). Ranked by
`ts_rank`, merged and re-sorted across both queries. **Transactionally
consistent**: the generated column commits with the row it indexes,
so a search can never see a title/block that doesn't exist yet or miss
one that was just saved.

**`SuggestTitles`** — the BK-tree query directly (`internal/bktree`,
pure Levenshtein-distance metric tree, ROADMAP.md § Two fuzzy searches'
own "prunes by the triangle inequality in a metric space"). Unlike
`Search`, this reads an **in-memory snapshot** rebuilt on its own
cadence (`search.DefaultRefreshInterval`, 30s) — search.html's own
admitted gap, stated plainly rather than hidden: "the index has its own
rebuild cadence and may lag the write path." A page renamed or created
in the last refresh window may not appear in a suggestion yet, even
though `Search` (never index-lagged) already sees it. `max_distance`
defaults to 2 when unset or non-positive — `ROADMAP.md`'s own "construct
[the Levenshtein automaton] for k <= 2" bound, reused here as this RPC's
practical default.

---

## 2. REST mapping (`api-gateway`)

| Method | Path | RPC |
|---|---|---|
| `GET` | `/search?q={query}` | `Search` |
| `GET` | `/search/suggest?q={query}&max_distance={n}` | `SuggestTitles` |

```json
// GET /search?q=budget
{
  "hits": [
    { "page_id": "...", "page_title": "Performance budget", "rank": 0.6 },
    { "page_id": "...", "page_title": "Rollout plan", "block_id": "...", "snippet": "ship on <b>budget</b>-critical dates", "rank": 0.3 }
  ]
}

// GET /search/suggest?q=Performnace&max_distance=2
{
  "suggestions": [
    { "page_id": "...", "title": "Performance budget", "distance": 2 }
  ]
}
```

`hits`/`suggestions` are always `[]`, never `null`, when there's nothing
to report — this repo's existing convention (`docs/api/diagnostics.md`).
An empty `q` returns an empty result immediately, without ever reaching
Postgres or the BK-tree — not an error, since a search box's own natural
starting state is empty.

Auth is a **verified bearer token** (`ADR-013` §1): `Authorization: Bearer
<access token>`, checked by `api-gateway` against `auth-service`'s JWKS, with
the actor id taken from the token's `sub` claim. `X-Actor-Id` is no longer
read anywhere — it was a value the caller wrote and nobody checked.

Search results are still **not permission-filtered**: identity is verified
now, but spaces and roles are `v3.1.0` part two, so there is nothing to
filter *by* yet. Stated rather than hidden — and the distinction matters,
because "we know who you are" and "we know what you may see" are different
claims and only the first is true today.
