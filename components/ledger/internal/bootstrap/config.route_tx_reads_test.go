// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"reflect"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig_RouteTransactionalReadsToPrimary_FieldTag locks the rollout-flag
// field onto the exact env var name. The transactional-read routing rollout is
// gated by this flag; renaming the field or the tag silently disables the gate.
func TestConfig_RouteTransactionalReadsToPrimary_FieldTag(t *testing.T) {
	t.Parallel()

	field, found := reflect.TypeOf(Config{}).FieldByName("RouteTransactionalReadsToPrimary")
	require.True(t, found, "Config must have field RouteTransactionalReadsToPrimary")
	assert.Equal(t, "DB_TRANSACTION_ROUTE_TX_READS_TO_PRIMARY", field.Tag.Get("env"),
		"RouteTransactionalReadsToPrimary must read from DB_TRANSACTION_ROUTE_TX_READS_TO_PRIMARY")
	assert.Equal(t, reflect.Bool, field.Type.Kind(),
		"RouteTransactionalReadsToPrimary must be a bool")
}

// TestConfig_RouteTransactionalReadsToPrimary_EnvParsing verifies safe-by-default,
// tolerant parsing via the repo's SetConfigFromEnvVars mechanism:
//   - unset          => false (backwards-compatible; app must not break)
//   - "true"         => true  (explicit opt-in)
//   - "false"        => false (explicit opt-out)
//   - invalid value  => false (fallback; SetConfigFromEnvVars must not error/panic)
func TestConfig_RouteTransactionalReadsToPrimary_EnvParsing(t *testing.T) {
	// Note: t.Parallel() omitted because sub-tests use t.Setenv.

	tests := []struct {
		name  string
		set   bool
		value string
		want  bool
	}{
		{name: "unset_defaults_false", set: false, want: false},
		{name: "explicit_true", set: true, value: "true", want: true},
		{name: "explicit_false", set: true, value: "false", want: false},
		{name: "invalid_falls_back_false", set: true, value: "notabool", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: t.Parallel() omitted because t.Setenv is incompatible with parallel sub-tests.
			if tt.set {
				t.Setenv("DB_TRANSACTION_ROUTE_TX_READS_TO_PRIMARY", tt.value)
			} else if orig, had := os.LookupEnv("DB_TRANSACTION_ROUTE_TX_READS_TO_PRIMARY"); had {
				os.Unsetenv("DB_TRANSACTION_ROUTE_TX_READS_TO_PRIMARY")
				t.Cleanup(func() { os.Setenv("DB_TRANSACTION_ROUTE_TX_READS_TO_PRIMARY", orig) })
			}

			cfg := &Config{}
			err := libCommons.SetConfigFromEnvVars(cfg)
			require.NoError(t, err,
				"SetConfigFromEnvVars must never fail on a missing or invalid rollout flag")

			applyConfigDefaults(cfg)

			assert.Equal(t, tt.want, cfg.RouteTransactionalReadsToPrimary,
				"RouteTransactionalReadsToPrimary must resolve to %v for value %q (set=%v)",
				tt.want, tt.value, tt.set)
		})
	}
}
