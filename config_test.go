package eidasaudit_test

import (
	"testing"

	"github.com/go-quicktest/qt"
	"github.com/spf13/viper"

	eidasaudit "github.com/go-make-bytes/eidas-audit"
)

// bind returns a configuration bound from v, so a test can assert what a
// deployment gets when it sets an environment variable — and what it gets when
// it sets nothing.
func bind(t *testing.T, env map[string]string) *eidasaudit.Configuration {
	t.Helper()

	for k, val := range env {
		t.Setenv(k, val)
	}

	v := viper.New()
	cfg := eidasaudit.NewConfiguration()
	cfg.Bind("", v)

	qt.Assert(t, qt.IsNil(v.Unmarshal(cfg)))

	return cfg
}

func TestConfig_StreamMaxBytesDefault(t *testing.T) {
	// The stream is a replay copy of a record the database holds durably, so it
	// ships bounded: 128 MiB unless a deployment says otherwise.
	qt.Check(t, qt.Equals(bind(t, nil).StreamMaxBytes, int64(134217728)))
}

func TestConfig_StreamMaxBytesConfigurable(t *testing.T) {
	cfg := bind(t, map[string]string{"EIDAS_AUDIT_STREAM_MAX_BYTES": "8388608"})
	qt.Check(t, qt.Equals(cfg.StreamMaxBytes, int64(8388608)))
}

func TestConfig_StreamMaxBytesUnlimitedIsSayable(t *testing.T) {
	// 0 means unlimited, and a deployment must be able to ask for it.
	cfg := bind(t, map[string]string{"EIDAS_AUDIT_STREAM_MAX_BYTES": "0"})
	qt.Check(t, qt.Equals(cfg.StreamMaxBytes, int64(0)))
}
