// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tink-crypto/tink-go/v2/keyset"
	"github.com/tink-crypto/tink-go/v2/mac"
)

// newTestMACPrimitive builds a MACPrimitive directly from an HMAC-SHA256 keyset.
// The MAC keyset generator and NewMACPrimitive constructor were removed (search
// tokens use PRF; the kept MACPrimitive serves only the legacy read path via a
// composite literal), so the test constructs the primitive the same way.
func newTestMACPrimitive(t *testing.T) *MACPrimitive {
	t.Helper()

	handle, err := keyset.NewHandle(mac.HMACSHA256Tag256KeyTemplate())
	require.NoError(t, err)

	primitive, err := mac.New(handle)
	require.NoError(t, err)

	return &MACPrimitive{primitive: primitive}
}

func TestMACPrimitive_ComputeMAC(t *testing.T) {
	t.Parallel()

	t.Run("computes MAC for data", func(t *testing.T) {
		t.Parallel()

		primitive := newTestMACPrimitive(t)

		tag, err := primitive.ComputeMAC([]byte("data to authenticate"))

		require.NoError(t, err)
		assert.NotEmpty(t, tag)
	})

	t.Run("produces deterministic MAC", func(t *testing.T) {
		t.Parallel()

		primitive := newTestMACPrimitive(t)

		data := []byte("consistent data")

		tag1, err := primitive.ComputeMAC(data)
		require.NoError(t, err)

		tag2, err := primitive.ComputeMAC(data)
		require.NoError(t, err)

		assert.Equal(t, tag1, tag2)
	})

	t.Run("different data produces different MAC", func(t *testing.T) {
		t.Parallel()

		primitive := newTestMACPrimitive(t)

		tag1, err := primitive.ComputeMAC([]byte("data one"))
		require.NoError(t, err)

		tag2, err := primitive.ComputeMAC([]byte("data two"))
		require.NoError(t, err)

		assert.NotEqual(t, tag1, tag2)
	})

	t.Run("computes MAC for empty data", func(t *testing.T) {
		t.Parallel()

		primitive := newTestMACPrimitive(t)

		tag, err := primitive.ComputeMAC([]byte{})

		require.NoError(t, err)
		assert.NotEmpty(t, tag)
	})
}
