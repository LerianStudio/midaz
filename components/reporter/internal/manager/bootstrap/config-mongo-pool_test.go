// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_MongoPoolFields_Exist(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		MongoMaxPoolSize:     "100",
		MongoMinPoolSize:     "10",
		MongoMaxConnIdleTime: "60s",
	}

	assert.Equal(t, "100", cfg.MongoMaxPoolSize)
	assert.Equal(t, "10", cfg.MongoMinPoolSize)
	assert.Equal(t, "60s", cfg.MongoMaxConnIdleTime)
}

func TestConfig_MongoPoolFields_Defaults(t *testing.T) {
	t.Parallel()

	cfg := &Config{}

	assert.Equal(t, "", cfg.MongoMaxPoolSize)
	assert.Equal(t, "", cfg.MongoMinPoolSize)
	assert.Equal(t, "", cfg.MongoMaxConnIdleTime)
}

func TestConfig_Validate_MongoMaxPoolSize_InvalidRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		poolSize    string
		expectedErr string
	}{
		{
			name:        "max pool size exceeds upper bound",
			poolSize:    "10001",
			expectedErr: "MONGO_MAX_POOL_SIZE",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := validManagerConfig()
			cfg.MongoMaxPoolSize = tt.poolSize

			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestConfig_Validate_MongoMinPoolSize_ExceedsMax(t *testing.T) {
	t.Parallel()

	cfg := validManagerConfig()
	cfg.MongoMaxPoolSize = "50"
	cfg.MongoMinPoolSize = "100" // min > max is invalid

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MONGO_MIN_POOL_SIZE")
}
