// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package contextutil

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntegrationIdentityValid(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		id    string
		valid bool
	}{
		{name: "simple", id: "ledger", valid: true},
		{name: "unicode", id: "razão-ledger", valid: true},
		{name: "at bound", id: strings.Repeat("a", maxIntegrationIDBytes), valid: true},
		{name: "empty", id: ""},
		{name: "over bound", id: strings.Repeat("a", maxIntegrationIDBytes+1)},
		{name: "leading space", id: " ledger"},
		{name: "trailing newline", id: "ledger\n"},
		{name: "NUL", id: "led\x00ger"},
		{name: "inner control", id: "led\x1bger"},
		{name: "invalid UTF-8", id: string([]byte{0xff})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.valid, IntegrationIdentity{ID: tc.id}.Valid())
		})
	}
}

func TestIntegrationIdentityContextRoundTrip(t *testing.T) {
	t.Parallel()

	identity, ok := GetIntegrationIdentity(WithIntegrationIdentity(context.Background(), IntegrationIdentity{ID: "ledger"}))
	require.True(t, ok)
	require.Equal(t, IntegrationIdentity{ID: "ledger"}, identity)

	identity, ok = GetIntegrationIdentity(WithIntegrationIdentity(context.Background(), IntegrationIdentity{ID: " ledger"}))
	require.False(t, ok, "an invalid identity attached to the context is never returned")
	require.Zero(t, identity)

	_, ok = GetIntegrationIdentity(context.Background())
	require.False(t, ok)

	var nilCtx context.Context

	_, ok = GetIntegrationIdentity(nilCtx)
	require.False(t, ok)
}
