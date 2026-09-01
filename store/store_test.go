package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-quicktest/qt"

	"github.com/gmb-lib/go-platform-kit/broker"

	"github.com/go-make-bytes/eidas-audit/store"
)

func ev(id string) *broker.Envelope {
	return &broker.Envelope{
		EventID:    id,
		OccurredAt: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		EventType:  "signing.applied",
		Categories: []broker.Category{broker.CategorySigning},
		Outcome:    broker.OutcomeSuccess,
	}
}

func TestChainHash_DeterministicAndPrevSensitive(t *testing.T) {
	e := ev("e1")

	h1, err := store.ChainHash(e, "")
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(len(h1), 64)) // hex SHA-256

	h1again, _ := store.ChainHash(e, "")
	qt.Check(t, qt.Equals(h1again, h1)) // deterministic

	h2, _ := store.ChainHash(e, "deadbeef")
	qt.Check(t, qt.IsTrue(h2 != h1)) // prev_hash changes the link
}

func TestMemory_AppendBuildsChain(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()

	r1, err := m.Append(ctx, ev("e1"), "eparaksts-signer")
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.IsFalse(r1.Duplicate))

	head, err := m.ChainHead(ctx)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(head.Hash, r1.Hash))
	qt.Check(t, qt.Equals(head.Seq, int64(1)))

	r2, err := m.Append(ctx, ev("e2"), "eparaksts-signer")
	qt.Assert(t, qt.IsNil(err))

	// Second event links to the first.
	got2, err := m.Get(ctx, "e2")
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(got2.PrevHash, r1.Hash))
	qt.Check(t, qt.Equals(got2.Hash, r2.Hash))

	// And its hash is reproducible from the chain inputs.
	want2, _ := store.ChainHash(ev("e2"), r1.Hash)
	qt.Check(t, qt.Equals(r2.Hash, want2))
}

func TestMemory_DuplicateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()

	r1, err := m.Append(ctx, ev("e1"), "")
	qt.Assert(t, qt.IsNil(err))

	r1dup, err := m.Append(ctx, ev("e1"), "")
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.IsTrue(r1dup.Duplicate))
	qt.Check(t, qt.Equals(r1dup.Hash, r1.Hash))

	// The chain did not advance.
	head, _ := m.ChainHead(ctx)
	qt.Check(t, qt.Equals(head.Seq, int64(1)))
}

func TestMemory_GetMissing(t *testing.T) {
	got, err := store.NewMemory().Get(context.Background(), "nope")
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.IsNil(got))
}
