// Package events emits the eidas-audit service's own NIS2-audit operational
// security events through go-sec-events. The consumer runs as a background Task
// with no azugo request context, so every event here takes the security library's
// background path — same shape as a request-path event, minus the correlation ids
// there is no request to take.
//
// NOTE: this is the service's *operational* telemetry (consume failures, chain
// integrity). The eIDAS-audit signing-evidence EVENTS the service stores are a
// different thing — they arrive from go-eidas-audit producers over the broker and
// are persisted, not emitted here.
package events

import (
	"context"

	"go.uber.org/zap"

	"github.com/gmb-lib/go-platform-kit/broker"
	"github.com/gmb-lib/go-sec-events/secevents"
)

// Event types emitted by the eidas-audit service.
const (
	EventConsumeFailed = "eidas_audit.consume_failed"
	EventChainMismatch = "eidas_audit.chain_mismatch"
	EventConsumerError = "eidas_audit.consumer_error"
)

// Emitter emits background (context-less) security events.
type Emitter struct {
	sec *secevents.Emitter
	log *zap.Logger
}

// New returns an Emitter delivering through the go-sec-events log sink. The sink
// carries log because this service emits only from background work, which has no
// request whose logger it could borrow.
func New(log *zap.Logger) *Emitter {
	if log == nil {
		log = zap.NewNop()
	}

	return &Emitter{sec: secevents.NewEmitter(secevents.NewLogSinkFor(log)), log: log}
}

// emit delivers one background security event.
func (e *Emitter) emit(eventType string, sev secevents.Severity, outcome broker.Outcome, attrs map[string]any) {
	if e == nil || e.sec == nil {
		return
	}
	if attrs == nil {
		attrs = map[string]any{}
	}
	attrs[secevents.AttrSeverity] = string(sev)

	ev := &broker.Envelope{
		EventType:  eventType,
		Categories: []broker.Category{broker.CategorySecurity},
		Outcome:    outcome,
		Attributes: attrs,
	}

	if err := e.sec.EmitBackground(context.Background(), ev); err != nil {
		e.log.Error("security event emission failed", zap.String("event_type", eventType), zap.Error(err))
	}
}

// ConsumeFailed records that an event could not be persisted (it will be
// redelivered). Warning severity — transient by assumption.
func (e *Emitter) ConsumeFailed(eventID, reason string) {
	e.emit(EventConsumeFailed, secevents.SeverityWarning, broker.OutcomeFailure, map[string]any{
		"event_id": eventID,
		"reason":   reason,
	})
}

// ChainMismatch records a hash-chain linkage rejection — high severity, a tamper
// or bug signal if it persists across redeliveries.
func (e *Emitter) ChainMismatch(eventID, detail string) {
	e.emit(EventChainMismatch, secevents.SeverityHigh, broker.OutcomeFailure, map[string]any{
		"event_id": eventID,
		"detail":   detail,
	})
}

// ConsumerError records a consumer-loop / broker error (connection, subscription).
func (e *Emitter) ConsumerError(detail string) {
	e.emit(EventConsumerError, secevents.SeverityHigh, broker.OutcomeFailure, map[string]any{
		"detail": detail,
	})
}
