// Package consumer is the eidas-audit broker consumer, packaged as an azugo
// core.Tasker so it runs either as this service's own background task or, bundled,
// inside another azugo service that imports it and AddTask()s it (the "no load at
// first" deployment option). It binds a durable JetStream pull consumer on the
// audit.signing subject via go-platform-kit/broker/natsbroker and lands each event
// into the hash-chained store.
package consumer

import (
	"context"
	"strings"
	"time"

	"azugo.io/core"
	"go.uber.org/zap"

	"github.com/gmb-lib/go-platform-kit/broker"
	"github.com/gmb-lib/go-platform-kit/broker/natsbroker"

	"github.com/go-make-bytes/eidas-audit/events"
	"github.com/go-make-bytes/eidas-audit/store"
)

// Task is an azugo core.Tasker.
var _ core.Tasker = (*Task)(nil)

// Config wires the consumer task's dependencies.
type Config struct {
	// Broker connection (from the platform BROKER_* config).
	BrokerURL string
	TLSCert   string
	TLSKey    string
	TLSCA     string
	// ServiceName is the NATS connection name (for broker-side monitoring).
	ServiceName string

	// Stream is ensured over StreamSubjects; the durable consumer reads Subject.
	Stream          string
	StreamSubjects  []string
	Subject         string
	Durable         string
	DuplicateWindow time.Duration
	// StreamMaxBytes caps the stream's size on disk (0 = unlimited); the oldest
	// messages are discarded at the cap. The store is the durable record — the
	// stream is the replay copy — so it is bounded on purpose.
	StreamMaxBytes int64

	// SourceService annotates each stored event (the publishing service). The
	// broker envelope carries no service field, so this is set from deployment
	// context where known, else left empty.
	SourceService string

	Store  store.Store
	Events *events.Emitter
	Logger *zap.Logger
}

// Task is the consumer as a core.Tasker.
type Task struct {
	cfg  Config
	conn *natsbroker.Conn
	cons *natsbroker.Consumer
}

// NewTask returns the consumer task.
func NewTask(cfg Config) *Task { return &Task{cfg: cfg} }

// Name identifies the task.
func (t *Task) Name() string { return "eidas-audit-consumer" }

// Start connects to NATS, ensures the stream, creates the durable consumer, and
// begins consuming. ctx scopes the dispatched work (it flows into store calls).
func (t *Task) Start(ctx context.Context) error {
	conn, err := natsbroker.Connect(natsbroker.Config{
		URL:     t.cfg.BrokerURL,
		TLSCert: t.cfg.TLSCert,
		TLSKey:  t.cfg.TLSKey,
		TLSCA:   t.cfg.TLSCA,
		Name:    t.cfg.ServiceName,
	})
	if err != nil {
		return err
	}
	t.conn = conn

	if err := conn.EnsureStream(ctx, natsbroker.StreamConfig{
		Name:       t.cfg.Stream,
		Subjects:   t.cfg.StreamSubjects,
		Duplicates: t.cfg.DuplicateWindow,
		MaxBytes:   t.cfg.StreamMaxBytes,
	}); err != nil {
		conn.Close()

		return err
	}

	cons, err := natsbroker.NewConsumer(ctx, conn, natsbroker.ConsumerConfig{
		Stream:        t.cfg.Stream,
		Durable:       t.cfg.Durable,
		FilterSubject: t.cfg.Subject,
	}, broker.NewMemoryIdempotencyStore(), t.handle, t.logger())
	if err != nil {
		conn.Close()

		return err
	}
	t.cons = cons

	if err := cons.Start(ctx); err != nil {
		conn.Close()

		return err
	}

	t.logger().Info("eidas-audit consumer started",
		zap.String("stream", t.cfg.Stream),
		zap.String("subject", t.cfg.Subject),
		zap.String("durable", t.cfg.Durable),
		zap.Int64("stream_max_bytes", t.cfg.StreamMaxBytes),
	)

	return nil
}

// Stop halts consumption and closes the connection.
func (t *Task) Stop() {
	if t.cons != nil {
		t.cons.Stop()
	}
	if t.conn != nil {
		t.conn.Close()
	}
}

// handle persists one event into the hash-chained store. Returning an error naks
// the message so JetStream redelivers (an audit event is never silently dropped);
// the store's event_id idempotency makes a redelivery safe.
func (t *Task) handle(ctx context.Context, ev *broker.Envelope) error {
	res, err := t.cfg.Store.Append(ctx, ev, t.cfg.SourceService)
	if err != nil {
		if strings.Contains(err.Error(), "chain_mismatch") {
			t.cfg.Events.ChainMismatch(ev.EventID, err.Error())
		} else {
			t.cfg.Events.ConsumeFailed(ev.EventID, err.Error())
		}

		return err
	}

	t.logger().Debug("audit event persisted",
		zap.String("event_id", res.EventID),
		zap.String("event_type", ev.EventType),
		zap.Bool("duplicate", res.Duplicate),
	)

	return nil
}

func (t *Task) logger() *zap.Logger {
	if t.cfg.Logger != nil {
		return t.cfg.Logger
	}

	return zap.NewNop()
}
