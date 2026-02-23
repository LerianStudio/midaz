// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package publisher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSecurityOptions(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		opts, err := buildSecurityOptions(SecurityConfig{})
		require.NoError(t, err)
		assert.Empty(t, opts)
	})

	t.Run("sasl enabled without credentials", func(t *testing.T) {
		_, err := buildSecurityOptions(SecurityConfig{SASLEnabled: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "username/password")
	})

	t.Run("unsupported mechanism", func(t *testing.T) {
		_, err := buildSecurityOptions(SecurityConfig{
			SASLEnabled:   true,
			SASLUsername:  "user",
			SASLPassword:  "pass",
			SASLMechanism: "INVALID",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported")
	})
}
