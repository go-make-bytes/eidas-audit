package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gmb-lib/go-platform-kit/broker"
)

// Postgres is the platform store: the append-only, hash-chained signing-evidence
// log in the `eidas_audit` schema, reached ONLY through SECURITY DEFINER
// procedures under the EXECUTE-only `eidas_audit_public` role (defined in
// authbyte-db/eidas-audit). This package never issues raw table SQL — it only
// CALLs the procedures (mirrors access-audit/store and trust-anchor/store).
//
// Selected when EIDAS_AUDIT_STORE_DSN is set; the in-memory backend is the
// development/test default. Append runs lock → read head → append as ONE database
// transaction (see Append), so replicas never race each other's chain; mu additionally
// keeps a single replica's own appends in order without queueing on the database.
type Postgres struct {
	pool *pgxpool.Pool
	mu   sync.Mutex
}

// NewPostgres opens a connection pool to the platform PostgreSQL. The pool is
// lazy; connectivity is verified on first use (or via Ping).
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: postgres connect: %w", err)
	}

	return &Postgres{pool: pool}, nil
}

// Close releases the connection pool.
func (p *Postgres) Close() { p.pool.Close() }

// Ping verifies backend connectivity.
func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// envelope is the structured result every procedure returns
// (util.result_success / util.result_error).
type envelope struct {
	Result  string          `json:"result"`
	Data    json.RawMessage `json:"data"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
}

// querier is what a procedure call needs from its connection: the pool for a
// standalone call, or an open transaction when several calls must be atomic.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// call invokes a SECURITY DEFINER procedure on the pool.
func (p *Postgres) call(ctx context.Context, proc string, in any) (json.RawMessage, error) {
	return callOn(ctx, p.pool, proc, in)
}

// callOn invokes a SECURITY DEFINER procedure with the uniform JSONB envelope on q
// and returns the decoded `data` payload, or a typed error from result_error.
func callOn(ctx context.Context, q querier, proc string, in any) (json.RawMessage, error) {
	inJSON, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("store: marshal input: %w", err)
	}

	// CALL with an INOUT parameter returns a single-column row carrying po_data;
	// NULL seeds the INOUT slot.
	stmt := fmt.Sprintf("CALL %s($1::jsonb, NULL::jsonb)", proc)

	var out []byte
	if err := q.QueryRow(ctx, stmt, inJSON).Scan(&out); err != nil {
		// A procedure that fails after a write re-raises a structured error with
		// SQLSTATE P0001 (Pattern B) to force a rollback; its message is the
		// util.result_error JSON. Recover the code/message so callers see the same
		// shape as the validation (returned-error) path.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "P0001" {
			var env envelope
			if json.Unmarshal([]byte(pgErr.Message), &env) == nil && env.Result == "error" {
				return nil, fmt.Errorf("store: %s: %s: %s", proc, env.Code, env.Message)
			}
		}

		return nil, fmt.Errorf("store: %s: %w", proc, err)
	}

	var env envelope
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, fmt.Errorf("store: %s: decode result: %w", proc, err)
	}
	if env.Result != "success" {
		return nil, fmt.Errorf("store: %s: %s: %s", proc, env.Code, env.Message)
	}

	return env.Data, nil
}

type appendInput struct {
	Event         *broker.Envelope `json:"event"`
	PrevHash      string           `json:"prev_hash"`
	Hash          string           `json:"hash"`
	SourceService string           `json:"source_service,omitempty"`
}

// Append computes the next chain link and persists ev via eidas_audit.append_event
// (idempotent on event id).
//
// Read-head and append happen in ONE transaction that first takes the chain's append
// lock (eidas_audit.lock_chain — the same lock append_event takes, re-entrant within
// the transaction). Without it two appenders read the same head and the second is
// refused as a chain_mismatch; with several replicas that turns into a redelivery
// storm in which most attempts fail. With it, appends queue on the lock and every one
// links correctly. mu keeps this replica's own appends in order without that queueing.
func (p *Postgres) Append(ctx context.Context, ev *broker.Envelope, sourceService string) (*AppendResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: append: begin: %w", err)
	}
	// Rollback after a successful Commit is a no-op; on any early return it releases the lock.
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := callOn(ctx, tx, "eidas_audit.lock_chain", map[string]any{}); err != nil {
		return nil, err
	}

	head, err := chainHeadOn(ctx, tx)
	if err != nil {
		return nil, err
	}

	prev := ""
	if head != nil {
		prev = head.Hash
	}

	hash, err := ChainHash(ev, prev)
	if err != nil {
		return nil, err
	}

	data, err := callOn(ctx, tx, "eidas_audit.append_event", appendInput{
		Event:         ev,
		PrevHash:      prev,
		Hash:          hash,
		SourceService: sourceService,
	})
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("store: append: commit: %w", err)
	}

	var res struct {
		RowID     string `json:"rowId"`
		EventID   string `json:"eventId"`
		Hash      string `json:"hash"`
		Duplicate bool   `json:"duplicate"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("store: append: decode: %w", err)
	}

	return &AppendResult{RowID: res.RowID, EventID: res.EventID, Hash: res.Hash, Duplicate: res.Duplicate}, nil
}

// ChainHead returns the current chain head via eidas_audit.chain_head.
func (p *Postgres) ChainHead(ctx context.Context) (*Head, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	return chainHeadOn(ctx, p.pool)
}

// chainHeadOn reads the current head on q — the pool for a plain read, the append
// transaction when the read must be atomic with the append that follows it.
func chainHeadOn(ctx context.Context, q querier) (*Head, error) {
	data, err := callOn(ctx, q, "eidas_audit.chain_head", map[string]any{})
	if err != nil {
		return nil, err
	}

	var res struct {
		Head *struct {
			Hash    string `json:"hash"`
			Seq     int64  `json:"seq"`
			EventID string `json:"eventId"`
		} `json:"head"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("store: chain_head: decode: %w", err)
	}
	if res.Head == nil {
		return nil, nil
	}

	return &Head{Hash: res.Head.Hash, Seq: res.Head.Seq, EventID: res.Head.EventID}, nil
}

// Get returns one event by event id via eidas_audit.get_event, or (nil, nil).
func (p *Postgres) Get(ctx context.Context, eventID string) (*Event, error) {
	data, err := p.call(ctx, "eidas_audit.get_event", map[string]any{"event_id": eventID})
	if err != nil {
		// not_found is a clean miss, not a failure.
		if isNotFound(err) {
			return nil, nil
		}

		return nil, err
	}

	var res struct {
		RowID    string          `json:"rowId"`
		Seq      int64           `json:"seq"`
		EventID  string          `json:"eventId"`
		PrevHash string          `json:"prevHash"`
		Hash     string          `json:"hash"`
		Content  json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("store: get: decode: %w", err)
	}

	var ev broker.Envelope
	if err := json.Unmarshal(res.Content, &ev); err != nil {
		return nil, fmt.Errorf("store: get: decode content: %w", err)
	}

	return &Event{
		RowID:    res.RowID,
		Seq:      res.Seq,
		Envelope: ev,
		PrevHash: res.PrevHash,
		Hash:     res.Hash,
	}, nil
}

// isNotFound recognises the eidas_audit:not_found result code that call() folds
// into its error string.
func isNotFound(err error) bool {
	return err != nil && containsNotFound(err.Error())
}

func containsNotFound(s string) bool {
	const code = "eidas_audit:not_found"
	for i := 0; i+len(code) <= len(s); i++ {
		if s[i:i+len(code)] == code {
			return true
		}
	}

	return false
}
