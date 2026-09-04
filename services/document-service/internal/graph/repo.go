// Package graph is GraphService's translation layer: load the live link
// graph from Postgres (docs.pages, docs.page_links), run
// graphalgo's pure algorithms over it, translate the result to
// documentv1's proto types. No algorithm lives in this package — that's
// graphalgo's whole reason to exist as its own dependency-free package;
// this one only does I/O and wire translation, the same split
// internal/pages already draws between repo.go and api.go.
package graph

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"marginal/document-service/internal/graphrepo/gen"
	"marginal/graphalgo"
)

// Node is one page as graph.LoadGraph reads it: identity, title, and
// whether it's a root (no parent) — graphalgo.Orphans' own root
// set is built from exactly this flag.
type Node struct {
	ID     uuid.UUID
	Title  string
	IsRoot bool
	// Topic is the page's declared classification as an ID, "" when
	// untopiced — the partition graphalgo.Modularity scores the wiring
	// against.
	Topic string
	// The same classification as a client draws it. Carried here rather than
	// joined by the client from ListPages, which returns one parent's
	// children and therefore covered only the root pages.
	TopicName     string
	TopicColorKey string
	Tags          []string
}

// LinkGraph is what LoadGraph returns: the graphalgo.Graph ready to feed
// into any of that package's algorithms, plus the node metadata
// (title, is_root) graphalgo's own NodeID-keyed maps don't carry.
type LinkGraph struct {
	Graph graphalgo.Graph
	Nodes map[graphalgo.NodeID]Node
	Roots []graphalgo.NodeID
}

// PostgresRepo is GraphService's only port onto docs.pages/docs.page_links.
type PostgresRepo struct {
	q *graphrepo.Queries
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{q: graphrepo.New(pool)}
}

// LoadGraph reads every live page and every resolved [[link]] and builds
// the in-memory graph graphalgo operates on. Two queries, not a join —
// a page with zero links still needs to appear as a Graph node (so
// orphan detection can see it sitting alone), which an INNER JOIN against
// page_links would silently drop.
// spaceIDs is REQUIRED, not optional: every query below filters on it, and
// an empty slice yields an empty graph rather than the whole instance —
// which is the safe direction for a mistake to fail in.
func (r *PostgresRepo) LoadGraph(ctx context.Context, spaceIDs []uuid.UUID) (LinkGraph, error) {
	scope := pgUUIDs(spaceIDs)
	pageRows, err := r.q.ListPagesForGraph(ctx, scope)
	if err != nil {
		return LinkGraph{}, fmt.Errorf("graph: loading pages: %w", err)
	}
	linkRows, err := r.q.ListResolvedLinksForGraph(ctx, scope)
	if err != nil {
		return LinkGraph{}, fmt.Errorf("graph: loading links: %w", err)
	}

	g := LinkGraph{
		Nodes: make(map[graphalgo.NodeID]Node, len(pageRows)),
	}
	g.Graph.Nodes = make([]graphalgo.NodeID, 0, len(pageRows))
	for _, p := range pageRows {
		id := graphalgo.NodeID(uuid.UUID(p.ID.Bytes).String())
		isRoot := !p.ParentID.Valid
		g.Graph.Nodes = append(g.Graph.Nodes, id)
		topic := ""
		if p.TopicID.Valid {
			topic = uuid.UUID(p.TopicID.Bytes).String()
		}
		topicName, topicColor := "", ""
		if p.TopicName != nil {
			topicName = *p.TopicName
		}
		if p.TopicColorKey != nil {
			topicColor = *p.TopicColorKey
		}
		g.Nodes[id] = Node{
			ID: uuid.UUID(p.ID.Bytes), Title: p.Title, IsRoot: isRoot,
			Topic: topic, TopicName: topicName, TopicColorKey: topicColor,
			Tags: p.Tags,
		}
		if isRoot {
			g.Roots = append(g.Roots, id)
		}
	}

	g.Graph.Edges = make([]graphalgo.Edge, 0, len(linkRows))
	for _, l := range linkRows {
		from := graphalgo.NodeID(uuid.UUID(l.FromPage.Bytes).String())
		to := graphalgo.NodeID(uuid.UUID(l.TargetPage.Bytes).String())
		g.Graph.Edges = append(g.Graph.Edges, graphalgo.Edge{From: from, To: to})
	}

	return g, nil
}

// pgUUIDs converts a space set for the ANY(...) predicate every query in
// this package now carries.
func pgUUIDs(ids []uuid.UUID) []pgtype.UUID {
	out := make([]pgtype.UUID, len(ids))
	for i, id := range ids {
		out[i] = pgtype.UUID{Bytes: id, Valid: true}
	}
	return out
}

// DanglingLink is one [[link]] whose target does not exist — § 20's check.
type DanglingLink struct {
	TargetTitle   string
	FromPage      uuid.UUID
	FromPageTitle string
	FromBlock     uuid.UUID
}

func (r *PostgresRepo) DanglingLinks(ctx context.Context, spaceIDs []uuid.UUID) ([]DanglingLink, error) {
	rows, err := r.q.ListDanglingLinks(ctx, pgUUIDs(spaceIDs))
	if err != nil {
		return nil, fmt.Errorf("graph: loading dangling links: %w", err)
	}
	out := make([]DanglingLink, 0, len(rows))
	for _, l := range rows {
		out = append(out, DanglingLink{
			TargetTitle: l.TargetTitle, FromPage: uuid.UUID(l.FromPage.Bytes),
			FromPageTitle: l.FromPageTitle, FromBlock: uuid.UUID(l.FromBlock.Bytes),
		})
	}
	return out, nil
}
