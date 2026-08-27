//go:build integration

package blockproj_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"marginal/documentcore"

	"marginal/document-service/internal/blockproj"
	"marginal/document-service/internal/migrate"
)

// Real Postgres via testcontainers-go, never a mock — the standing rule
// for anything touching infrastructure (.agents/agents.md). Requires
// Docker; run with: go test -tags=integration ./internal/blockproj/...
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("document_service_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, migrate.Up(sqlDB))

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func createTestPage(t *testing.T, pool *pgxpool.Pool, title string) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	ltreeLabel := "p" + strings.ReplaceAll(id.String(), "-", "")
	_, err := pool.Exec(context.Background(),
		`INSERT INTO docs.pages (id, created_by, title, path, sort_key) VALUES ($1, $2, $3, $4::ltree, 'a')`,
		id, actor, title, ltreeLabel)
	require.NoError(t, err)
	return id
}

func blockOpJSON(t *testing.T, op documentcore.Op) []byte {
	t.Helper()
	inner, err := documentcore.MarshalOp(op)
	require.NoError(t, err)
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(inner, &m))
	scopeJSON, _ := json.Marshal("block")
	m["scope"] = scopeJSON
	data, err := json.Marshal(m)
	require.NoError(t, err)
	return data
}

func textOpJSON(t *testing.T, blockID documentcore.BlockID, opType, text string) []byte {
	t.Helper()
	payload := map[string]any{
		"scope": "text",
		"block": blockID.String(),
		"op":    map[string]any{"type": opType, "text": text},
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return data
}

func listBlocks(t *testing.T, pool *pgxpool.Pool, pageID uuid.UUID) []struct {
	ID       uuid.UUID
	Kind     string
	Text     string
	ParentID *uuid.UUID
	Path     string
} {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT id, kind->>'tag', content->>'text', parent_id, path::text FROM docs.blocks WHERE page_id = $1 ORDER BY position`, pageID)
	require.NoError(t, err)
	defer rows.Close()

	var out []struct {
		ID       uuid.UUID
		Kind     string
		Text     string
		ParentID *uuid.UUID
		Path     string
	}
	for rows.Next() {
		var r struct {
			ID       uuid.UUID
			Kind     string
			Text     string
			ParentID *uuid.UUID
			Path     string
		}
		require.NoError(t, rows.Scan(&r.ID, &r.Kind, &r.Text, &r.ParentID, &r.Path))
		out = append(out, r)
	}
	return out
}

func TestInsertBlockThenTextOpMaterializesContent(t *testing.T) {
	pool := newTestPool(t)
	pageID := createTestPage(t, pool, "Test Page")
	proj := blockproj.New(pool)
	ctx := context.Background()

	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{
		ID: blockID, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: ""},
	})))
	require.NoError(t, proj.HandleEvent(ctx, pageID, textOpJSON(t, blockID, "InsertText", "hello world")))

	blocks := listBlocks(t, pool, pageID)
	require.Len(t, blocks, 1)
	assert.Equal(t, "paragraph", blocks[0].Kind)
	assert.Equal(t, "hello world", blocks[0].Text)
}

// TestDeleteTextAndNoOpDoNotWipeProjectedText pins a real bug: applyTextOp
// used to treat any non-InsertText op (including NoOp, and DeleteText
// before its paired InsertText arrives) as "clear this block's text to
// empty." A DeleteText that isn't immediately followed by a real
// InsertText (e.g. a delete-only op the client never completed with a
// non-empty replacement) or a genuine NoOp event left the projection
// permanently blank with no later event to correct it.
func TestDeleteTextAndNoOpDoNotWipeProjectedText(t *testing.T) {
	pool := newTestPool(t)
	pageID := createTestPage(t, pool, "Test Page 5")
	proj := blockproj.New(pool)
	ctx := context.Background()

	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{
		ID: blockID, Kind: documentcore.NewParagraph(),
	})))
	require.NoError(t, proj.HandleEvent(ctx, pageID, textOpJSON(t, blockID, "InsertText", "hello")))

	require.NoError(t, proj.HandleEvent(ctx, pageID, textOpJSON(t, blockID, "DeleteText", "")))
	blocks := listBlocks(t, pool, pageID)
	require.Len(t, blocks, 1)
	assert.Equal(t, "hello", blocks[0].Text, "a bare DeleteText must not wipe the projected text")

	require.NoError(t, proj.HandleEvent(ctx, pageID, textOpJSON(t, blockID, "NoOp", "")))
	blocks = listBlocks(t, pool, pageID)
	require.Len(t, blocks, 1)
	assert.Equal(t, "hello", blocks[0].Text, "a NoOp must not wipe the projected text")
}

// TestCalloutWithChildMaterialisesInProjection confirms RFC-001 §10's
// Callout/Aside containers project the same as Quote/Toggle (§1) — a
// child under a Callout gets the correct parent_id, and Callout's own
// kind-specific fields (tone, icon) round-trip through docs.blocks.kind.
func TestCalloutWithChildMaterialisesInProjection(t *testing.T) {
	pool := newTestPool(t)
	pageID := createTestPage(t, pool, "Callout Page")
	proj := blockproj.New(pool)
	ctx := context.Background()

	callout := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{
		ID: callout, Kind: documentcore.NewCallout(documentcore.Danger, "⚠️"),
	})))
	child := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{
		ID: child, Parent: &callout, Kind: documentcore.NewParagraph(),
	})))
	require.NoError(t, proj.HandleEvent(ctx, pageID, textOpJSON(t, child, "InsertText", "be careful")))

	blocks := listBlocks(t, pool, pageID)
	require.Len(t, blocks, 2)
	assert.Equal(t, "callout", blocks[0].Kind)
	require.NotNil(t, blocks[1].ParentID)
	assert.Equal(t, uuid.UUID(callout), *blocks[1].ParentID)
	assert.Equal(t, "be careful", blocks[1].Text)

	var toneAndIcon struct {
		Tone string `json:"tone"`
		Icon string `json:"icon"`
	}
	require.NoError(t, pool.QueryRow(ctx, `SELECT kind->>'tone', kind->>'icon' FROM docs.blocks WHERE id = $1`, callout).
		Scan(&toneAndIcon.Tone, &toneAndIcon.Icon))
	assert.Equal(t, "danger", toneAndIcon.Tone)
	assert.Equal(t, "⚠️", toneAndIcon.Icon)
}

// TestInsertBlockUnderContainerMaterialisesParentIDAndPath pins RFC-001
// §1's containment through the whole projection: a child block's
// parent_id and its LTREE path (the quote's own path, plus the child's
// label) must both land in docs.blocks, not just its position order.
func TestInsertBlockUnderContainerMaterialisesParentIDAndPath(t *testing.T) {
	pool := newTestPool(t)
	pageID := createTestPage(t, pool, "Nested Page")
	proj := blockproj.New(pool)
	ctx := context.Background()

	quote := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{
		ID: quote, Kind: documentcore.NewQuote(),
	})))
	child := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{
		ID: child, Parent: &quote, Kind: documentcore.NewParagraph(),
	})))
	require.NoError(t, proj.HandleEvent(ctx, pageID, textOpJSON(t, child, "InsertText", "inside the quote")))

	blocks := listBlocks(t, pool, pageID)
	require.Len(t, blocks, 2)
	assert.Equal(t, uuid.UUID(quote), blocks[0].ID)
	assert.Nil(t, blocks[0].ParentID)
	assert.Equal(t, uuid.UUID(child), blocks[1].ID)
	require.NotNil(t, blocks[1].ParentID)
	assert.Equal(t, uuid.UUID(quote), *blocks[1].ParentID)
	assert.Equal(t, "inside the quote", blocks[1].Text)
	assert.True(t, strings.HasPrefix(blocks[1].Path, blocks[0].Path+"."),
		"child's path %q must extend the parent's own path %q", blocks[1].Path, blocks[0].Path)
}

// TestMoveBlockReparentsWholeSubtreeInProjection confirms the same
// whole-subtree relocation Page.Apply's own MoveBlock case implements
// (documentcore/page_test.go's TestMoveBlockRelocatesWholeSubtreeAsOneUnit)
// survives the projector round trip: a container's child stays put,
// still parented to it, after the container itself moves.
func TestMoveBlockReparentsWholeSubtreeInProjection(t *testing.T) {
	pool := newTestPool(t)
	pageID := createTestPage(t, pool, "Reparent Page")
	proj := blockproj.New(pool)
	ctx := context.Background()

	quote := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: quote, Kind: documentcore.NewQuote()})))
	child := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: child, Parent: &quote, Kind: documentcore.NewParagraph()})))
	target := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: target, After: &quote, Kind: documentcore.NewToggle()})))

	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.MoveBlock{
		ID: quote, FromParent: nil, From: nil, ToParent: &target, To: nil,
	})))

	blocks := listBlocks(t, pool, pageID)
	require.Len(t, blocks, 3)
	assert.Equal(t, []uuid.UUID{uuid.UUID(target), uuid.UUID(quote), uuid.UUID(child)},
		[]uuid.UUID{blocks[0].ID, blocks[1].ID, blocks[2].ID})
	require.NotNil(t, blocks[1].ParentID)
	assert.Equal(t, uuid.UUID(target), *blocks[1].ParentID, "the moved subtree's own root is reparented")
	require.NotNil(t, blocks[2].ParentID)
	assert.Equal(t, uuid.UUID(quote), *blocks[2].ParentID, "its child keeps its existing parent")
}

// TestProjectorRehydratesNestingFromPersistedState confirms a fresh
// Projector (a process restart) reconstructs Parent from parent_id
// correctly, not just each block's own kind/content — the nested
// counterpart to TestProjectorRehydratesFromPersistedStateAfterRestart.
func TestProjectorRehydratesNestingFromPersistedState(t *testing.T) {
	pool := newTestPool(t)
	pageID := createTestPage(t, pool, "Rehydrate Nested Page")
	ctx := context.Background()

	first := blockproj.New(pool)
	quote := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, first.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: quote, Kind: documentcore.NewQuote()})))
	child := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, first.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: child, Parent: &quote, Kind: documentcore.NewParagraph()})))

	// A brand new Projector, as a process restart would create — it must
	// rehydrate quote/child's parent relationship from docs.blocks before
	// accepting a further op that depends on it (inserting a second
	// child after the first, which needs to know the first's real
	// parent/position to place the new one correctly).
	second := blockproj.New(pool)
	grandchild := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, second.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: grandchild, Parent: &quote, After: &child, Kind: documentcore.NewParagraph()})))

	blocks := listBlocks(t, pool, pageID)
	require.Len(t, blocks, 3)
	assert.Equal(t, []uuid.UUID{uuid.UUID(quote), uuid.UUID(child), uuid.UUID(grandchild)},
		[]uuid.UUID{blocks[0].ID, blocks[1].ID, blocks[2].ID})
	require.NotNil(t, blocks[2].ParentID)
	assert.Equal(t, uuid.UUID(quote), *blocks[2].ParentID)
}

func TestSetBlockKindAndMoveBlockReflectInProjection(t *testing.T) {
	pool := newTestPool(t)
	pageID := createTestPage(t, pool, "Test Page 2")
	proj := blockproj.New(pool)
	ctx := context.Background()

	a := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	b := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: a, Kind: documentcore.NewParagraph()})))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: b, After: &a, Kind: documentcore.NewParagraph()})))

	heading, err := documentcore.NewHeading(2)
	require.NoError(t, err)
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.SetBlockKind{ID: a, From: documentcore.NewParagraph(), To: heading})))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.MoveBlock{ID: a, From: nil, To: &b})))

	blocks := listBlocks(t, pool, pageID)
	require.Len(t, blocks, 2)
	assert.Equal(t, b, documentcore.BlockID(blocks[0].ID), "b should now be first")
	assert.Equal(t, a, documentcore.BlockID(blocks[1].ID), "a moved after b")
	assert.Equal(t, "heading", blocks[1].Kind)
}

func TestDeleteBlockRemovesItFromProjection(t *testing.T) {
	pool := newTestPool(t)
	pageID := createTestPage(t, pool, "Test Page 3")
	proj := blockproj.New(pool)
	ctx := context.Background()

	a := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: a, Kind: documentcore.NewParagraph()})))
	require.Len(t, listBlocks(t, pool, pageID), 1)

	require.NoError(t, proj.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.DeleteBlock{
		Tombstone: documentcore.Block{ID: a, Kind: documentcore.NewParagraph()}, After: nil,
	})))
	assert.Empty(t, listBlocks(t, pool, pageID))
}

func TestPageLinkResolvesWhenTargetExistsAndDanglesWhenItDoesnt(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	target := createTestPage(t, pool, "Target Page")
	source := createTestPage(t, pool, "Source Page")
	proj := blockproj.New(pool)

	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, proj.HandleEvent(ctx, source, blockOpJSON(t, documentcore.InsertBlock{ID: blockID, Kind: documentcore.NewParagraph()})))
	require.NoError(t, proj.HandleEvent(ctx, source, textOpJSON(t, blockID, "InsertText", "see [[Target Page]] and [[Missing Page]]")))

	rows, err := pool.Query(ctx, `SELECT target_title, target_page FROM docs.page_links WHERE from_page = $1 ORDER BY target_title`, source)
	require.NoError(t, err)
	defer rows.Close()

	type link struct {
		Title      string
		TargetPage *uuid.UUID
	}
	var links []link
	for rows.Next() {
		var l link
		require.NoError(t, rows.Scan(&l.Title, &l.TargetPage))
		links = append(links, l)
	}
	require.Len(t, links, 2)
	assert.Equal(t, "Missing Page", links[0].Title)
	assert.Nil(t, links[0].TargetPage, "a link to a page that doesn't exist must be dangling")
	assert.Equal(t, "Target Page", links[1].Title)
	require.NotNil(t, links[1].TargetPage)
	assert.Equal(t, target, *links[1].TargetPage)
}

func TestProjectorRehydratesFromPersistedStateAfterRestart(t *testing.T) {
	pool := newTestPool(t)
	pageID := createTestPage(t, pool, "Test Page 4")
	ctx := context.Background()

	first := blockproj.New(pool)
	a := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, first.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: a, Kind: documentcore.NewParagraph()})))
	require.NoError(t, first.HandleEvent(ctx, pageID, textOpJSON(t, a, "InsertText", "hello")))

	// A brand new Projector, as a process restart would create — it must
	// rehydrate this page's state from docs.blocks (never from collab.ops,
	// which this service can't reach) before applying the next op.
	second := blockproj.New(pool)
	b := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	require.NoError(t, second.HandleEvent(ctx, pageID, blockOpJSON(t, documentcore.InsertBlock{ID: b, After: &a, Kind: documentcore.NewParagraph()})))
	require.NoError(t, second.HandleEvent(ctx, pageID, textOpJSON(t, b, "InsertText", "world")))

	blocks := listBlocks(t, pool, pageID)
	require.Len(t, blocks, 2, "the first block must survive a fresh Projector rehydrating from docs.blocks")
	assert.Equal(t, "hello", blocks[0].Text)
	assert.Equal(t, "world", blocks[1].Text)
}
