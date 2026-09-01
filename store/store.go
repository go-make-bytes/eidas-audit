// Package store persists the append-only, hash-chained eIDAS signing-evidence log
// (eIDAS-audit). The platform backend is PostgreSQL reached ONLY through the
// `eidas_audit` schema's SECURITY DEFINER procedures under an EXECUTE-only role
// (authbyte-db/eidas-audit); an in-memory backend exists for tests/dev. No backend
// exposes raw table access — every operation is a procedure call.
//
// The hash-chain is built HERE in Go: Append reads the current
// chain head, computes hash = SHA-256(canonical(envelope) || prev_hash), and hands
// prev_hash + hash to the append procedure, which verifies the linkage before
// inserting. The Postgres backend runs lock → read head → append as one database
// transaction (eidas_audit.lock_chain first), so appenders on different replicas
// queue on the chain instead of racing it; an in-process mutex additionally keeps a
// single replica's own appends in order. A chain_mismatch therefore signals a real
// fault (tampering or a bug), not ordinary concurrency.
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gmb-lib/go-platform-kit/broker"
)

// errInvalid is returned by a backend when a required field is missing.
var errInvalid = errors.New("store: event missing event_id")

// Event is one persisted signing-evidence event: the producer's broker envelope
// plus the chain links and server annotation.
type Event struct {
	// RowID is the store-assigned ULID primary key (populated on read).
	RowID string
	// Seq is the DB-assigned monotonic chain/insert order (read side).
	Seq int64
	// Envelope is the producer's event, stored verbatim as the hashed content.
	Envelope broker.Envelope
	// PrevHash is the previous row's Hash ("" for the genesis event).
	PrevHash string
	// Hash is SHA-256(canonical(Envelope) || PrevHash), hex-encoded.
	Hash string
	// SourceService is the publishing service (annotation; may be empty).
	SourceService string
}

// AppendResult reports the outcome of an append.
type AppendResult struct {
	RowID     string
	EventID   string
	Hash      string
	Duplicate bool
}

// Head is the current chain head.
type Head struct {
	Hash    string
	Seq     int64
	EventID string
}

// Store is the signing-evidence persistence contract. It maps onto the
// eidas_audit schema procedures.
type Store interface {
	// Append computes the next chain link for ev and persists it, idempotent on
	// ev.EventID (a JetStream redelivery returns Duplicate=true, not an error).
	// sourceService is the publishing service annotation (may be "").
	Append(ctx context.Context, ev *broker.Envelope, sourceService string) (*AppendResult, error)
	// ChainHead returns the current chain head, or (nil, nil) for an empty chain.
	ChainHead(ctx context.Context) (*Head, error)
	// Get returns one event (verbatim envelope + chain links) by event id, or
	// (nil, nil) when absent.
	Get(ctx context.Context, eventID string) (*Event, error)
	// Ping verifies backend connectivity for readiness checks.
	Ping(ctx context.Context) error
	// Close releases backend resources.
	Close()
}

// ChainHash computes the chain hash for ev linked to prevHash:
// hex(SHA-256(canonical(ev) || prevHash)), where canonical(ev) is the JSON
// encoding of the envelope (deterministic: struct fields in declaration order,
// map keys sorted by encoding/json). The same computation re-run over a stored
// event's content + prev_hash re-derives the hash, so a verifier can walk the
// chain. NOTE (skeleton limitation, see DECISIONS): canonicalisation relies on a
// lossless Envelope round-trip; a stronger canonical form is future work.
func ChainHash(ev *broker.Envelope, prevHash string) (string, error) {
	canonical, err := json.Marshal(ev)
	if err != nil {
		return "", fmt.Errorf("store: canonicalise envelope: %w", err)
	}

	h := sha256.New()
	h.Write(canonical)
	h.Write([]byte(prevHash))

	return hex.EncodeToString(h.Sum(nil)), nil
}
