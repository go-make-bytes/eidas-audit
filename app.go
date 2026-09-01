// Package eidasaudit is the eSignature-Portal eidas-audit service: the eIDAS-audit
// (eIDAS/ETSI signing evidence) sink. It consumes the append-only broker.Envelope
// stream on the audit.signing subject (published by go-eidas-audit producers such
// as eparaksts-signer) and lands each event into its own append-only, hash-chained
// `eidas_audit` store, proving who applied which signature to which document, when,
// at what assurance level.
//
// It is NOT the go-eidas-audit library (the emitter) and NOT the access-audit
// service (the GDPR-audit GDPR sink); its schema `eidas_audit` is separate from
// `access_audit`. The consumer is a self-contained azugo Task (consumer.Task), so
// the same code runs standalone here or bundled inside another azugo service.
package eidasaudit

import (
	"azugo.io/azugo"
	"azugo.io/azugo/server"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/gmb-lib/go-platform-kit/platform"

	"github.com/go-make-bytes/eidas-audit/consumer"
	"github.com/go-make-bytes/eidas-audit/events"
	"github.com/go-make-bytes/eidas-audit/store"
)

// App is the eidas-audit application container.
type App struct {
	*azugo.App

	config *Configuration

	store  store.Store
	events *events.Emitter
}

// New creates the application: configuration, platform cross-cutting setup, the
// signing-evidence store, and (when a broker is configured) the consumer task.
func New(cmd *cobra.Command, version string) (*App, error) {
	config := NewConfiguration()

	a, err := server.New(cmd, server.Options{
		AppName:       "eIDAS Audit & Evidence Service",
		AppVer:        version,
		Configuration: config,
	})
	if err != nil {
		return nil, err
	}

	app := &App{App: a, config: config}
	if err := app.init(); err != nil {
		return nil, err
	}

	return app, nil
}

func (a *App) init() error {
	cfg := a.config

	if err := platform.Setup(a.App, platform.Options{
		Config: cfg.BaseConfiguration,
	}); err != nil {
		return err
	}

	a.events = events.New(a.Log())

	var err error
	switch cfg.StoreBackend() {
	case StoreBackendPostgres:
		a.store, err = store.NewPostgres(a.BackgroundContext(), cfg.StoreDSN)
		if err != nil {
			return err
		}
	default:
		a.Log().Warn("no store DSN configured (EIDAS_AUDIT_STORE_DSN) — using in-memory store; audit events will NOT survive restarts (development only)")
		a.store = store.NewMemory()
	}

	// The consumer is the durable eIDAS-audit sink. Without a broker the service
	// still serves health (e.g. a pure-dev run); the consumer is wired only when
	// BROKER_URL is set.
	if url := cfg.BrokerURL(); url != "" {
		broker := cfg.Broker

		return a.AddTask(consumer.NewTask(consumer.Config{
			BrokerURL:       url,
			TLSCert:         broker.TLSCert,
			TLSKey:          broker.TLSKey,
			TLSCA:           broker.TLSCA,
			ServiceName:     cfg.ServiceName,
			Stream:          cfg.Stream,
			StreamSubjects:  cfg.StreamSubjects,
			Subject:         cfg.Subject,
			Durable:         cfg.Durable,
			DuplicateWindow: cfg.DuplicateWindow,
			StreamMaxBytes:  cfg.StreamMaxBytes,
			Store:           a.store,
			Events:          a.events,
			Logger:          a.Log(),
		}))
	}

	a.Log().Warn("no broker configured (BROKER_URL) — the eidas-audit consumer is NOT running; only health endpoints are served (development only)")

	return nil
}

// Start verifies store connectivity (non-fatal) then starts the server and the
// consumer task.
func (a *App) Start() error {
	if err := a.store.Ping(a.BackgroundContext()); err != nil {
		a.Log().Warn("eidas-audit store not reachable at start — readiness will report not-ready until it recovers", zap.Error(err))
	}

	return a.App.Start()
}

// Config returns the loaded configuration.
func (a *App) Config() *Configuration {
	if a.config == nil || !a.config.Ready() {
		panic("configuration is not loaded")
	}

	return a.config
}

// Store returns the signing-evidence store.
func (a *App) Store() store.Store { return a.store }

// Events returns the security-event emitter.
func (a *App) Events() *events.Emitter { return a.events }
