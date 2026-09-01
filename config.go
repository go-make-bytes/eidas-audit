package eidasaudit

import (
	"time"

	azugocfg "azugo.io/azugo/config"
	"azugo.io/core/validation"
	"github.com/spf13/viper"

	pkconfig "github.com/gmb-lib/go-platform-kit/config"
)

// Store backends.
const (
	StoreBackendPostgres = "postgres"
	StoreBackendMemory   = "memory"
)

// Configuration is the eidas-audit service configuration: the squashed platform
// base configuration (which carries SERVICE_NAME + the BROKER_* connection) plus
// the eidas-audit-specific store + JetStream settings.
//
// There is no inbound-auth sub-config: the consume path is broker-driven and the
// only HTTP surface is unauthenticated healthz/readyz. (The future DSAR/verify
// read API will add go-authbyte with audience svc:eidas-audit.)
type Configuration struct {
	*pkconfig.BaseConfiguration `mapstructure:",squash"`

	// StoreDSN selects + configures the PostgreSQL backend (the `eidas_audit`
	// schema reached via SECURITY DEFINER procedures under the EXECUTE-only
	// `eidas_audit_public` role). When set it is used; otherwise the in-memory
	// backend is used (development/test only — events do NOT survive restarts).
	// Source it from Vault in production (it carries a password).
	StoreDSN string `mapstructure:"eidas_audit_store_dsn"`

	// Stream is the JetStream stream name the consumer binds (and ensures exists).
	Stream string `mapstructure:"eidas_audit_stream" validate:"required"`
	// StreamSubjects are the subjects the stream captures (the consumer ensures
	// the stream over these). Defaults to the audit.> space.
	StreamSubjects []string `mapstructure:"eidas_audit_stream_subjects" validate:"required,min=1"`
	// Subject is the filter subject the durable consumer reads (the go-eidas-audit
	// DefaultTopic, "audit.signing").
	Subject string `mapstructure:"eidas_audit_subject" validate:"required"`
	// Durable is the durable consumer name (so the cursor survives restarts).
	Durable string `mapstructure:"eidas_audit_durable" validate:"required"`
	// DuplicateWindow is the JetStream server-side Msg-Id dedup window for the
	// stream (a backstop beneath the DB event_id idempotency).
	DuplicateWindow time.Duration `mapstructure:"eidas_audit_duplicate_window" validate:"required,gt=0"`
	// StreamMaxBytes bounds the stream's size on disk (0 = unlimited). The
	// database holds the durable, hash-chained record; the stream is the copy
	// kept to replay events that a restore would otherwise lose, so it is sized
	// by the replay window a deployment wants rather than by total volume. At the
	// cap the oldest messages are discarded and publishing keeps succeeding.
	StreamMaxBytes int64 `mapstructure:"eidas_audit_stream_max_bytes" validate:"gte=0"`
}

// NewConfiguration returns the configuration skeleton for binding.
func NewConfiguration() *Configuration {
	return &Configuration{BaseConfiguration: pkconfig.New()}
}

// ServerCore returns the embedded azugo configuration.
func (c *Configuration) ServerCore() *azugocfg.Configuration {
	return c.Configuration
}

// Bind registers defaults and environment bindings.
func (c *Configuration) Bind(_ string, v *viper.Viper) {
	c.BaseConfiguration.Bind("", v)

	v.SetDefault("eidas_audit_stream", "AUDIT")
	v.SetDefault("eidas_audit_stream_subjects", []string{"audit.>"})
	v.SetDefault("eidas_audit_subject", "audit.signing")
	v.SetDefault("eidas_audit_durable", "eidas-audit")
	v.SetDefault("eidas_audit_duplicate_window", 2*time.Minute)
	v.SetDefault("eidas_audit_stream_max_bytes", int64(134217728)) // 128 MiB

	_ = v.BindEnv("eidas_audit_store_dsn", "EIDAS_AUDIT_STORE_DSN")
	_ = v.BindEnv("eidas_audit_stream", "EIDAS_AUDIT_STREAM")
	_ = v.BindEnv("eidas_audit_stream_subjects", "EIDAS_AUDIT_STREAM_SUBJECTS")
	_ = v.BindEnv("eidas_audit_subject", "EIDAS_AUDIT_SUBJECT")
	_ = v.BindEnv("eidas_audit_durable", "EIDAS_AUDIT_DURABLE")
	_ = v.BindEnv("eidas_audit_duplicate_window", "EIDAS_AUDIT_DUPLICATE_WINDOW")
	_ = v.BindEnv("eidas_audit_stream_max_bytes", "EIDAS_AUDIT_STREAM_MAX_BYTES")
}

// Validate validates the configuration.
func (c *Configuration) Validate(valid *validation.Validate) error {
	if err := c.BaseConfiguration.Validate(valid); err != nil {
		return err
	}

	return valid.Struct(c)
}

// StoreBackend derives the store backend from configuration.
func (c *Configuration) StoreBackend() string {
	if c.StoreDSN != "" {
		return StoreBackendPostgres
	}

	return StoreBackendMemory
}

// BrokerURL returns the configured broker endpoint (BROKER_URL), or "" when the
// broker is not wired (the service then runs without the consumer).
func (c *Configuration) BrokerURL() string {
	if c.Broker == nil {
		return ""
	}

	return c.Broker.URL
}
