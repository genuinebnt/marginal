// Package pagesaga runs the page-delete saga ARCHITECTURE.md §5 describes,
// reduced to this repo's real participant set (§ What this actually is at
// this repo's scope): document-service does four steps in its own database,
// collaboration-service does one across a service boundary and acks it, and
// two more are named but have no store to act on until v4.
//
// The saga is choreographed — there is no coordinator. Each step is a pure
// function of (page, database) that is safe to run twice, and progress is a
// list of completed step names in docs.page_deletions.steps_done. Resume is
// therefore not a mechanism so much as a consequence: the sweeper re-runs
// from the first step not named, and a step that ran but crashed before
// recording itself simply runs again for free.
package pagesaga

// Step names as they are persisted in docs.page_deletions.steps_done and
// sent on the wire (docs/api/pages.md § SagaProgress). These strings are a
// stored format: a completed saga's row keeps them after a rename, so
// changing one is a migration, not a refactor.
const (
	// Soft-delete the page and its LTREE subtree in one transaction. First
	// because every later step is scoped to "pages this delete covers," and
	// that set has to be settled before anything reads it.
	StepTreeDetached = "tree_detached"

	// Rewrite docs.page_links rows that pointed AT the deleted subtree.
	// The links are not removed: a [[link]] to a deleted page is a real
	// dangling reference and RFC-003's DanglingLink analyzer is supposed to
	// see it. What this step drops is the rows where the deleted page was
	// the SOURCE, which are now claims made by a page that no longer exists.
	StepLinksRewritten = "links_rewritten"

	// Drop the deleted subtree from the FTS index. In-process here (the
	// index is a generated column in this service's own database, v2.5.0),
	// which is why this is one statement rather than a cross-service call —
	// but it stays a named step, because resumability does not depend on
	// where a step runs. A crash between "tree detached" and "index dropped"
	// still has to resume at the index.
	StepSearchIndex = "search_index"

	// The one genuinely remote step: publish docs.page_deleted and wait for
	// collaboration-service's collab.page_released ack. It holds a live rope
	// and an op log over this page, and purging rows out from under it is
	// the failure the whole saga exists to prevent.
	//
	// The op log itself is SEALED, never deleted — collab.ops is append-only
	// (DATA_MODEL.md), and a deleted page's history is exactly what restore
	// and audit need to still exist.
	StepSessionsReleased = "sessions_released"

	// Named, ordered, and reported — but with nothing to act on yet. There
	// is no vector store until v4.4.0 and no object store until v4.2.0, so
	// these complete immediately and say so rather than being silently
	// skipped.
	//
	// They are real steps rather than a TODO because the alternative is
	// worse in both directions: omitting them means the step list quietly
	// changes shape when v4 lands, and faking work means the trash screen
	// reports progress that never happened. Run returns NotApplicable for
	// them, which the UI renders as "no store yet" — the same honesty the
	// mockup applies to routes it has not drawn.
	StepEmbeddingsPurged = "embeddings_purged"
	StepBlobsReleased    = "blobs_released"
)

// Steps is the saga's order. The sweeper resumes at the first entry not
// present in steps_done, so this slice — not the database — is the
// authority on what "finished" means, and a release that appends a step
// automatically re-opens every saga that completed under the old list.
//
// That is deliberate: a page deleted before v4.4.0 genuinely does have
// embeddings to purge once embeddings exist. Ordering matters only where a
// step reads what an earlier one wrote (everything is scoped to the subtree
// StepTreeDetached settles); the rest is cheapest-first.
var Steps = []string{
	StepTreeDetached,
	StepLinksRewritten,
	StepSearchIndex,
	StepSessionsReleased,
	StepEmbeddingsPurged,
	StepBlobsReleased,
}

// Remaining returns the steps in Steps not present in done, in Steps order.
// The sweeper runs these; the API reports them as steps_left.
func Remaining(done []string) []string {
	seen := make(map[string]struct{}, len(done))
	for _, s := range done {
		seen[s] = struct{}{}
	}
	var left []string
	for _, s := range Steps {
		if _, ok := seen[s]; !ok {
			left = append(left, s)
		}
	}
	return left
}

// NotApplicable reports whether step has no backing store at this repo's
// scope, so callers can render it as such rather than as work performed.
// Kept as a function over the same constants rather than a field on a step
// struct: this is a fact about the repo's current scope that changes when a
// feature lands, not a property of the step itself.
func NotApplicable(step string) bool {
	return step == StepEmbeddingsPurged || step == StepBlobsReleased
}
