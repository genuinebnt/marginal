// Package pagesrest is pages.md §2's gateway REST mapping — a thin
// translation to/from document-service's PageService gRPC contract (§1 of
// that doc), which stays the source of truth for semantics.
package pagesrest

import (
	"time"

	documentv1 "marginal/document-service/genproto/documentv1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// pageJSON matches pages.md §2's example body field-for-field, including
// deleted_at's omission while active (documented there explicitly: "sent
// as null" would be wrong).
type pageJSON struct {
	ID             string  `json:"id"`
	CreatedBy      string  `json:"created_by"`
	Title          string  `json:"title"`
	ParentID       *string `json:"parent_id"`
	Path           string  `json:"path"`
	SortKey        string  `json:"sort_key"`
	LifecycleState string  `json:"lifecycle_state"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	DeletedAt      *string `json:"deleted_at,omitempty"`
	// v2.7.0. Topic is absent when the page is untopiced — a real state the
	// UI reports, so `null` here is meaningful rather than missing data.
	// Tags is always present, empty rather than null, so the client never
	// has to special-case absence before iterating.
	Topic *topicJSON `json:"topic"`
	Tags  []string   `json:"tags"`
	// v2.8.0. Always present, 0 rather than omitted: an empty page and a page
	// whose block projection has not caught up both genuinely hold zero
	// blocks right now, and omitting the field would make the client guess
	// which of the two it is looking at from the shape of the JSON.
	BlockCount int32 `json:"block_count"`
	WordCount  int32 `json:"word_count"`
}

type topicJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// A key into the design system's categorical ramp
	// (DESIGN_GUIDELINES.md §3.4), never a hex value — the palette is the
	// frontend's to own, and shipping colours over the wire would fork it.
	ColorKey  string `json:"color_key"`
	PageCount int32  `json:"page_count,omitempty"`
}

func toTopicJSON(t *documentv1.Topic) *topicJSON {
	if t == nil {
		return nil
	}
	return &topicJSON{ID: t.GetId(), Name: t.GetName(), ColorKey: t.GetColorKey(), PageCount: t.GetPageCount()}
}

func toPageJSON(p *documentv1.Page) pageJSON {
	out := pageJSON{
		ID:             p.GetId(),
		CreatedBy:      p.GetCreatedBy(),
		Title:          p.GetTitle(),
		Path:           p.GetPath(),
		SortKey:        p.GetSortKey(),
		LifecycleState: lifecycleStateJSON(p.GetLifecycleState()),
		CreatedAt:      formatTimestamp(p.GetCreatedAt()),
		UpdatedAt:      formatTimestamp(p.GetUpdatedAt()),
	}
	if p.ParentId != nil {
		out.ParentID = p.ParentId
	}
	if p.DeletedAt != nil {
		s := formatTimestamp(p.DeletedAt)
		out.DeletedAt = &s
	}
	out.Topic = toTopicJSON(p.GetTopic())
	out.Tags = p.GetTags()
	if out.Tags == nil {
		out.Tags = []string{}
	}
	out.BlockCount = p.GetBlockCount()
	out.WordCount = p.GetWordCount()
	return out
}

// lifecycleStateJSON renders documentv1's proto enum as the lowercase
// word pages.md's example JSON uses, not the generated PascalCase
// LIFECYCLE_STATE_* constant name.
func lifecycleStateJSON(s documentv1.LifecycleState) string {
	switch s {
	case documentv1.LifecycleState_LIFECYCLE_STATE_ACTIVE:
		return "active"
	case documentv1.LifecycleState_LIFECYCLE_STATE_DELETING:
		return "deleting"
	case documentv1.LifecycleState_LIFECYCLE_STATE_DELETED:
		return "deleted"
	default:
		return "unspecified"
	}
}

// formatTimestamp is pages.md §2's "RFC 3339 UTC — google.protobuf.Timestamp
// on the wire, formatted at the gateway."
func formatTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format(time.RFC3339Nano)
}

// backlinkJSON is one row of GET /pages/{id}/backlinks — deliberately not
// a pageJSON: a backlink only ever needs enough to render a row in the
// inspector (docs/ui-mockups/v2/index.html § 04 EDITOR's Backlinks tab), not a page's
// full metadata.
type backlinkJSON struct {
	FromPage        string `json:"from_page"`
	FromPageTitle   string `json:"from_page_title"`
	FromPageDeleted bool   `json:"from_page_deleted"`
	TargetTitle     string `json:"target_title"`
}

type listBacklinksJSON struct {
	Backlinks []backlinkJSON `json:"backlinks"`
}

func toListBacklinksJSON(resp *documentv1.ListBacklinksResponse) listBacklinksJSON {
	out := make([]backlinkJSON, len(resp.GetBacklinks()))
	for i, b := range resp.GetBacklinks() {
		out[i] = backlinkJSON{
			FromPage:        b.GetFromPage(),
			FromPageTitle:   b.GetFromPageTitle(),
			FromPageDeleted: b.GetFromPageDeleted(),
			TargetTitle:     b.GetTargetTitle(),
		}
	}
	return listBacklinksJSON{Backlinks: out}
}

type listPagesJSON struct {
	Pages      []pageJSON `json:"pages"`
	NextCursor *string    `json:"next_cursor,omitempty"`
}

func toListPagesJSON(resp *documentv1.ListPagesResponse) listPagesJSON {
	pages := make([]pageJSON, len(resp.GetPages()))
	for i, p := range resp.GetPages() {
		pages[i] = toPageJSON(p)
	}
	return listPagesJSON{Pages: pages, NextCursor: resp.NextCursor}
}

// --- series (v2.9.0) -------------------------------------------------------

type seriesPartJSON struct {
	PageID string `json:"page_id"`
	Title  string `json:"title"`
	// 1-based, as printed ("Part 3 of 19"). 0 is never a valid part number.
	Number    int32      `json:"number"`
	WordCount int32      `json:"word_count"`
	Topic     *topicJSON `json:"topic"`
	Tags      []string   `json:"tags"`
}

type pageSeriesJSON struct {
	// "none" | "member" | "leader" — three states that need different words
	// on screen, so they are three strings rather than two booleans.
	Membership   string           `json:"membership"`
	SeriesPageID string           `json:"series_page_id"`
	SeriesTitle  string           `json:"series_title"`
	Parts        []seriesPartJSON `json:"parts"`
	Number       int32            `json:"number"`
}

type seriesSummaryJSON struct {
	SeriesPageID string           `json:"series_page_id"`
	Title        string           `json:"title"`
	Topic        *topicJSON       `json:"topic"`
	PartCount    int32            `json:"part_count"`
	WordCount    int32            `json:"word_count"`
	Parts        []seriesPartJSON `json:"parts"`
}

type listSeriesJSON struct {
	Series []seriesSummaryJSON `json:"series"`
}

func membershipJSON(m documentv1.Membership) string {
	switch m {
	case documentv1.Membership_MEMBERSHIP_MEMBER:
		return "member"
	case documentv1.Membership_MEMBERSHIP_LEADER:
		return "leader"
	default:
		return "none"
	}
}

func toSeriesPartsJSON(in []*documentv1.SeriesPart) []seriesPartJSON {
	out := make([]seriesPartJSON, 0, len(in))
	for _, p := range in {
		tags := p.GetTags()
		if tags == nil {
			tags = []string{}
		}
		out = append(out, seriesPartJSON{
			PageID: p.GetPageId(), Title: p.GetTitle(), Number: p.GetNumber(),
			WordCount: p.GetWordCount(), Topic: toTopicJSON(p.GetTopic()), Tags: tags,
		})
	}
	return out
}

func toPageSeriesJSON(s *documentv1.PageSeries) pageSeriesJSON {
	return pageSeriesJSON{
		Membership:   membershipJSON(s.GetMembership()),
		SeriesPageID: s.GetSeriesPageId(),
		SeriesTitle:  s.GetSeriesTitle(),
		Parts:        toSeriesPartsJSON(s.GetParts()),
		Number:       s.GetNumber(),
	}
}

func toListSeriesJSON(r *documentv1.ListSeriesResponse) listSeriesJSON {
	out := listSeriesJSON{Series: make([]seriesSummaryJSON, 0, len(r.GetSeries()))}
	for _, s := range r.GetSeries() {
		out.Series = append(out.Series, seriesSummaryJSON{
			SeriesPageID: s.GetSeriesPageId(),
			Title:        s.GetTitle(),
			Topic:        toTopicJSON(s.GetTopic()),
			PartCount:    s.GetPartCount(),
			WordCount:    s.GetWordCount(),
			Parts:        toSeriesPartsJSON(s.GetParts()),
		})
	}
	return out
}

// --- trash (v2.6.0's saga, made visible) -----------------------------------

type sagaProgressJSON struct {
	StepsDone []string `json:"steps_done"`
	StepsLeft []string `json:"steps_left"`
	// > 1 means the saga resumed after a crash. Reported because that is the
	// difference between a slow delete and an unstable one.
	Attempts  int32   `json:"attempts"`
	LastError *string `json:"last_error,omitempty"`
	// Steps with no backing store at this repo's scope. Sent so the UI can
	// render them as "no store yet" rather than as work performed.
	NotApplicable []string `json:"not_applicable"`
}

type trashEntryJSON struct {
	Page    pageJSON `json:"page"`
	PurgeAt string   `json:"purge_at"`
	// Absent once the saga finishes: a finished saga has no progress to
	// report, so absence is how a caller tells 'deleted, restorable' from
	// 'deleting, mid-saga' without re-reading lifecycle_state.
	Progress *sagaProgressJSON `json:"progress,omitempty"`
}

type listTrashJSON struct {
	Entries []trashEntryJSON `json:"entries"`
	Total   int32            `json:"total"`
}

type deletePreviewJSON struct {
	Descendants []pageJSON `json:"descendants"`
	// Pages OUTSIDE the subtree whose [[links]] point into it. Those links do
	// not break — they DANGLE, which is a state the graph and the diagnostics
	// engine both already model.
	Referrers  []pageJSON `json:"referrers"`
	BlockCount int32      `json:"block_count"`
}

func toListTrashJSON(r *documentv1.ListTrashResponse) listTrashJSON {
	out := listTrashJSON{Total: r.GetTotal(), Entries: make([]trashEntryJSON, 0, len(r.GetEntries()))}
	for _, e := range r.GetEntries() {
		entry := trashEntryJSON{Page: toPageJSON(e.GetPage()), PurgeAt: formatTimestamp(e.GetPurgeAt())}
		if p := e.GetProgress(); p != nil {
			prog := &sagaProgressJSON{
				StepsDone:     emptyStrings(p.GetStepsDone()),
				StepsLeft:     emptyStrings(p.GetStepsLeft()),
				Attempts:      p.GetAttempts(),
				NotApplicable: emptyStrings(p.GetNotApplicable()),
			}
			if p.LastError != nil {
				msg := p.GetLastError()
				prog.LastError = &msg
			}
			entry.Progress = prog
		}
		out.Entries = append(out.Entries, entry)
	}
	return out
}

func toDeletePreviewJSON(r *documentv1.PreviewDeleteResponse) deletePreviewJSON {
	out := deletePreviewJSON{
		BlockCount:  r.GetBlockCount(),
		Descendants: make([]pageJSON, 0, len(r.GetDescendants())),
		Referrers:   make([]pageJSON, 0, len(r.GetReferrers())),
	}
	for _, p := range r.GetDescendants() {
		out.Descendants = append(out.Descendants, toPageJSON(p))
	}
	for _, p := range r.GetReferrers() {
		out.Referrers = append(out.Referrers, toPageJSON(p))
	}
	return out
}

// emptyStrings ships `[]` rather than `null` — a client iterating a step list
// should not need a guard for "the saga had none".
func emptyStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
