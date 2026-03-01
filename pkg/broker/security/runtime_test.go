// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNonProductionEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  string
		want bool
	}{
		{name: "development", env: "development", want: true},
		{name: "staging", env: "staging", want: true},
		{name: "ci", env: "ci", want: true},
		{name: "production", env: "production", want: false},
		{name: "empty", env: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsNonProductionEnvironment(tt.env))
		})
	}
}

func TestValidateRuntimeConfig(t *testing.T) {
	t.Parallel()

	t.Run("rejects insecure skip verify in production", func(t *testing.T) {
		t.Parallel()

		warnings, err := ValidateRuntimeConfig(RuntimeConfig{
			Environment:           "production",
			TLSEnabled:            true,
			TLSInsecureSkipVerify: true,
			SASLEnabled:           true,
		})

		require.Error(t, err)
		assert.Empty(t, warnings)
	})

	t.Run("warns for disabled tls and tls without sasl in production", func(t *testing.T) {
		t.Parallel()

		warnings, err := ValidateRuntimeConfig(RuntimeConfig{
			Environment:           "production",
			TLSEnabled:            false,
			TLSInsecureSkipVerify: false,
			SASLEnabled:           false,
		})

		require.NoError(t, err)
		assert.Contains(t, warnings, "TLS is disabled in a production-like environment")
	})

	t.Run("warns when tls is enabled without sasl in production", func(t *testing.T) {
		t.Parallel()

		warnings, err := ValidateRuntimeConfig(RuntimeConfig{
			Environment:           "production",
			TLSEnabled:            true,
			TLSInsecureSkipVerify: false,
			SASLEnabled:           false,
		})

		require.NoError(t, err)
		assert.Contains(t, warnings, "TLS is enabled without SASL authentication in a production-like environment")
	})

	t.Run("allows insecure skip verify in non production with warning", func(t *testing.T) {
		t.Parallel()

		warnings, err := ValidateRuntimeConfig(RuntimeConfig{
			Environment:           "development",
			TLSEnabled:            true,
			TLSInsecureSkipVerify: true,
			SASLEnabled:           false,
		})

		require.NoError(t, err)
		assert.Contains(t, warnings, "TLS_INSECURE_SKIP_VERIFY=true: server certificate verification is disabled")
	})
}
