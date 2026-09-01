package store

import (
	"context"
	"sync"

	"github.com/oklog/ulid/v2"

	"github.com/gmb-lib/go-platform-kit/broker"
)

// Memory is an in-process, non-durable Store for tests and development. It builds
// the same hash-chain as the Postgres backend (via ChainHash) but loses all state
// on restart — never use it in production (the service warns when it falls back to
// it). Safe for concurrent use.
type Memory struct {
	mu     sync.Mutex
	byID   map[string]*Event
	events []*Event // in chain order; the last element is the head
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{byID: make(map[string]*Event)}
}

// Append computes the next chain link for ev and stores it, idempotent on EventID.
func (m *Memory) Append(_ context.Context, ev *broker.Envelope, sourceService string) (*AppendResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ev == nil || ev.EventID == "" {
		return nil, errInvalid
	}

	if existing, ok := m.byID[ev.EventID]; ok {
		return &AppendResult{RowID: existing.RowID, EventID: ev.EventID, Hash: existing.Hash, Duplicate: true}, nil
	}

	prev := ""
	if n := len(m.events); n > 0 {
		prev = m.events[n-1].Hash
	}

	hash, err := ChainHash(ev, prev)
	if err != nil {
		return nil, err
	}

	rec := &Event{
		RowID:         ulid.Make().String(),
		Seq:           int64(len(m.events) + 1),
		Envelope:      *ev,
		PrevHash:      prev,
		Hash:          hash,
		SourceService: sourceService,
	}
	m.events = append(m.events, rec)
	m.byID[ev.EventID] = rec

	return &AppendResult{RowID: rec.RowID, EventID: ev.EventID, Hash: hash, Duplicate: false}, nil
}

// ChainHead returns the head of the chain, or (nil, nil) when empty.
func (m *Memory) ChainHead(_ context.Context) (*Head, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.events) == 0 {
		return nil, nil
	}

	h := m.events[len(m.events)-1]

	return &Head{Hash: h.Hash, Seq: h.Seq, EventID: h.Envelope.EventID}, nil
}

// Get returns one event by id, or (nil, nil).
func (m *Memory) Get(_ context.Context, eventID string) (*Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	rec, ok := m.byID[eventID]
	if !ok {
		return nil, nil
	}

	cp := *rec

	return &cp, nil
}

// Ping always succeeds.
func (m *Memory) Ping(_ context.Context) error { return nil }

// Close is a no-op.
func (m *Memory) Close() {}
