// Package opstore is collab.ops's Postgres side — the durable, queryable
// home a LoggedOp reaches after the local WAL (docs/porting/PROGRESS.md's
// batched-flush "Next") lands it here. Append is idempotent on the op's
// own id, since the flush loop that will call it operates at-least-once
// (RFC-002 §4 rule 5: OpId is the dedup key for exactly this reason).
package opstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"marginal/collaboration-service/internal/collabrepo/gen"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/pageop"
)

// OutboxEventOpAppended is collab.outbox's event_type for a flushed op —
// the one event this repo's scope currently produces (DATA_MODEL.md's
// collab.ops_flushed, the load-bearing event document-service will
// eventually consume to materialise docs.blocks; no consumer exists in
// this repo yet, see the migration's own comment).
const OutboxEventOpAppended = "collab.ops_flushed"

// Repo is opstore's only port — small, declared at its point of use
// (CLOUD_PORTABILITY.md). The Postgres implementation lives in this file.
type Repo interface {
	// Append persists l and its outbox event in one transaction, and
	// reports whether it was actually a new row: false means l.ID was
	// already present (a retried flush after a crash, not an error) and
	// no outbox event was written either — one write, one event, exactly
	// once from the caller's point of view even though the underlying
	// delivery is at-least-once.
	Append(ctx context.Context, l oplog.LoggedOp) (inserted bool, err error)
	// AppendBatch is Append for a whole flush interval's worth of ops at
	// once (internal/flush), in the same all-or-nothing transaction and
	// with the same per-op dedup semantics — but as two pipelined
	// round trips total (one Batch for the op rows, one for the matching
	// outbox events) instead of two per op. RFC-002 §7: batching is what
	// makes the write volume survivable, not a micro-optimization.
	// Returns how many of ls were actually new.
	AppendBatch(ctx context.Context, ls []oplog.LoggedOp) (inserted int, err error)
	// ListForPage returns every op ever appended for pageID, oldest
	// first — history replay's basic building block (RFC-002 §1).
	ListForPage(ctx context.Context, pageID uuid.UUID) ([]oplog.LoggedOp, error)
}

type PostgresRepo struct {
	q    *collabrepo.Queries
	pool *pgxpool.Pool
}

func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{q: collabrepo.New(pool), pool: pool}
}

// opRow is everything Append and AppendBatch both need computed from a
// LoggedOp before they can build either the single-statement or the
// batched sqlc params for it — one place that does the marshaling, so the
// two call paths can't drift on how a LoggedOp becomes a row.
type opRow struct {
	kind    string
	payload []byte
	clock   []byte
}

func buildOpRow(l oplog.LoggedOp) (opRow, error) {
	kind, err := pageop.TypeName(l.Op)
	if err != nil {
		return opRow{}, fmt.Errorf("%w", err)
	}
	payload, err := pageop.Marshal(l.Op)
	if err != nil {
		return opRow{}, fmt.Errorf("marshaling payload: %w", err)
	}
	clockJSON, err := marshalVectorClock(l.VectorClock)
	if err != nil {
		return opRow{}, fmt.Errorf("marshaling vector clock: %w", err)
	}
	return opRow{kind: kind, payload: payload, clock: clockJSON}, nil
}

func (r *PostgresRepo) Append(ctx context.Context, l oplog.LoggedOp) (bool, error) {
	row, err := buildOpRow(l)
	if err != nil {
		return false, fmt.Errorf("opstore: append: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("opstore: append: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	_, err = q.InsertOp(ctx, collabrepo.InsertOpParams{
		ID:              pgtype.UUID{Bytes: l.ID, Valid: true},
		PageID:          pgtype.UUID{Bytes: l.PageID, Valid: true},
		ActorID:         pgtype.UUID{Bytes: l.ActorID, Valid: true},
		ActorKind:       string(l.ActorKind),
		UndoGroup:       toPgUUIDPtr(l.UndoGroup),
		EncodingVersion: int16(l.Version),
		Kind:            row.kind,
		Payload:         row.payload,
		VectorClock:     row.clock,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING: this id was already flushed by an
		// earlier attempt — nothing new to commit, and no new outbox
		// event either (the first successful attempt already wrote one).
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("opstore: append: insert op: %w", err)
	}

	outboxID, err := uuid.NewV7()
	if err != nil {
		return false, fmt.Errorf("opstore: append: generating outbox id: %w", err)
	}
	if _, err := q.InsertOutboxEvent(ctx, collabrepo.InsertOutboxEventParams{
		ID:          pgtype.UUID{Bytes: outboxID, Valid: true},
		AggregateID: pgtype.UUID{Bytes: l.PageID, Valid: true},
		EventType:   OutboxEventOpAppended,
		Payload:     row.payload,
	}); err != nil {
		return false, fmt.Errorf("opstore: append: insert outbox event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("opstore: append: commit: %w", err)
	}
	return true, nil
}

// AppendBatch inserts ls as one pgx.Batch (op rows), determines which were
// actually new from that batch's own results, then inserts a second
// pgx.Batch (outbox events) for only those — two round trips regardless of
// len(ls), both inside one transaction so the whole flush interval commits
// or rolls back together.
func (r *PostgresRepo) AppendBatch(ctx context.Context, ls []oplog.LoggedOp) (int, error) {
	if len(ls) == 0 {
		return 0, nil
	}

	rows := make([]opRow, len(ls))
	opParams := make([]collabrepo.InsertOpBatchParams, len(ls))
	for i, l := range ls {
		row, err := buildOpRow(l)
		if err != nil {
			return 0, fmt.Errorf("opstore: append batch: op %d: %w", i, err)
		}
		rows[i] = row
		opParams[i] = collabrepo.InsertOpBatchParams{
			ID:              pgtype.UUID{Bytes: l.ID, Valid: true},
			PageID:          pgtype.UUID{Bytes: l.PageID, Valid: true},
			ActorID:         pgtype.UUID{Bytes: l.ActorID, Valid: true},
			ActorKind:       string(l.ActorKind),
			UndoGroup:       toPgUUIDPtr(l.UndoGroup),
			EncodingVersion: int16(l.Version),
			Kind:            row.kind,
			Payload:         row.payload,
			VectorClock:     row.clock,
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("opstore: append batch: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	var newlyInserted []int // indices into ls
	var firstErr error
	opResults := q.InsertOpBatch(ctx, opParams)
	opResults.QueryRow(func(i int, _ collabrepo.CollabOp, err error) {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// this id was already flushed by an earlier attempt
		case err != nil:
			if firstErr == nil {
				firstErr = err
			}
		default:
			newlyInserted = append(newlyInserted, i)
		}
	})
	if closeErr := opResults.Close(); closeErr != nil && firstErr == nil {
		firstErr = closeErr
	}
	if firstErr != nil {
		return 0, fmt.Errorf("opstore: append batch: insert ops: %w", firstErr)
	}

	if len(newlyInserted) > 0 {
		outboxParams := make([]collabrepo.InsertOutboxEventBatchParams, len(newlyInserted))
		for j, i := range newlyInserted {
			outboxID, err := uuid.NewV7()
			if err != nil {
				return 0, fmt.Errorf("opstore: append batch: generating outbox id: %w", err)
			}
			outboxParams[j] = collabrepo.InsertOutboxEventBatchParams{
				ID:          pgtype.UUID{Bytes: outboxID, Valid: true},
				AggregateID: pgtype.UUID{Bytes: ls[i].PageID, Valid: true},
				EventType:   OutboxEventOpAppended,
				Payload:     rows[i].payload,
			}
		}

		var outboxErr error
		outboxResults := q.InsertOutboxEventBatch(ctx, outboxParams)
		outboxResults.QueryRow(func(_ int, _ collabrepo.CollabOutbox, err error) {
			if err != nil && outboxErr == nil {
				outboxErr = err
			}
		})
		if closeErr := outboxResults.Close(); closeErr != nil && outboxErr == nil {
			outboxErr = closeErr
		}
		if outboxErr != nil {
			return 0, fmt.Errorf("opstore: append batch: insert outbox events: %w", outboxErr)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("opstore: append batch: commit: %w", err)
	}
	return len(newlyInserted), nil
}

func (r *PostgresRepo) ListForPage(ctx context.Context, pageID uuid.UUID) ([]oplog.LoggedOp, error) {
	rows, err := r.q.ListOpsForPage(ctx, pgtype.UUID{Bytes: pageID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("opstore: list for page: %w", err)
	}
	out := make([]oplog.LoggedOp, len(rows))
	for i, row := range rows {
		l, err := loggedOpFromRow(row)
		if err != nil {
			return nil, fmt.Errorf("opstore: list for page: decoding row %d: %w", i, err)
		}
		out[i] = l
	}
	return out, nil
}

func loggedOpFromRow(row collabrepo.CollabOp) (oplog.LoggedOp, error) {
	op, err := pageop.Unmarshal(row.Payload)
	if err != nil {
		return oplog.LoggedOp{}, fmt.Errorf("unmarshaling op: %w", err)
	}
	clock, err := unmarshalVectorClock(row.VectorClock)
	if err != nil {
		return oplog.LoggedOp{}, fmt.Errorf("unmarshaling vector clock: %w", err)
	}
	return oplog.LoggedOp{
		ID:          row.ID.Bytes,
		Version:     uint16(row.EncodingVersion),
		PageID:      row.PageID.Bytes,
		ActorID:     row.ActorID.Bytes,
		ActorKind:   oplog.ActorKind(row.ActorKind),
		UndoGroup:   fromPgUUIDPtr(row.UndoGroup),
		VectorClock: clock,
		Op:          op,
		CreatedAt:   row.CreatedAt.Time,
	}, nil
}

// marshalVectorClock defaults a nil clock to "{}" rather than JSON null —
// collab.ops.vector_clock is NOT NULL at the column level either way (a
// JSON null literal is still a non-NULL column value), but an empty object
// is the honest representation of "no clock entries yet."
func marshalVectorClock(c oplog.VectorClock) ([]byte, error) {
	if c == nil {
		c = oplog.VectorClock{}
	}
	return json.Marshal(c)
}

func unmarshalVectorClock(data []byte) (oplog.VectorClock, error) {
	var c oplog.VectorClock
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return c, nil
}

func toPgUUIDPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func fromPgUUIDPtr(id pgtype.UUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	u := uuid.UUID(id.Bytes)
	return &u
}
