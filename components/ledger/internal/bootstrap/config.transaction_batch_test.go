// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"os"
	"reflect"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_TransactionBatchMaxSizeField(t *testing.T) {
	t.Parallel()

	field, found := reflect.TypeOf(Config{}).FieldByName("TransactionBatchMaxSize")
	require.True(t, found)
	assert.Equal(t, "TRANSACTION_BATCH_MAX_SIZE", field.Tag.Get("env"))
	assert.Equal(t, reflect.Int, field.Type.Kind())
}

func TestConfig_TransactionBatchMaxSize(t *testing.T) {
	// t.Parallel is intentionally omitted because subtests mutate the process environment.
	tests := []struct {
		name            string
		set             bool
		value           string
		want            int
		wantConfigError string
	}{
		{name: "unset defaults to 10", want: 10},
		{name: "blank defaults to 10", set: true, value: "", want: 10},
		{name: "lower bound", set: true, value: "1", want: 1},
		{name: "operational default", set: true, value: "10", want: 10},
		{name: "upper bound", set: true, value: "50", want: 50},
		{name: "explicit zero is rejected", set: true, value: "0", wantConfigError: "between 1 and 50, got 0"},
		{name: "negative value is rejected", set: true, value: "-1", want: -1, wantConfigError: "between 1 and 50, got -1"},
		{name: "value above ceiling is rejected", set: true, value: "51", want: 51, wantConfigError: "between 1 and 50, got 51"},
		{name: "non integer is rejected", set: true, value: "invalid", wantConfigError: "between 1 and 50, got 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.set {
				t.Setenv("TRANSACTION_BATCH_MAX_SIZE", tt.value)
			} else {
				unsetEnvForTest(t, "TRANSACTION_BATCH_MAX_SIZE")
			}

			cfg := &Config{}
			err := libCommons.SetConfigFromEnvVars(cfg)
			require.NoError(t, err)
			applyConfigDefaults(cfg)
			assert.Equal(t, tt.want, cfg.TransactionBatchMaxSize)

			err = validateTransactionBatchConfig(cfg)
			if tt.wantConfigError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantConfigError)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestInitServersWithOptions_RejectsInvalidTransactionBatchMaxSizeBeforeInfrastructure(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "above ceiling", value: "51", want: "TRANSACTION_BATCH_MAX_SIZE must be between 1 and 50, got 51"},
		{name: "non integer", value: "invalid", want: "TRANSACTION_BATCH_MAX_SIZE must be between 1 and 50, got 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TRANSACTION_BATCH_MAX_SIZE", tt.value)

			service, err := InitServersWithOptions(nil)

			require.Error(t, err)
			assert.Nil(t, service)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()

	value, found := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))

	t.Cleanup(func() {
		if found {
			require.NoError(t, os.Setenv(key, value))

			return
		}

		require.NoError(t, os.Unsetenv(key))
	})
}
