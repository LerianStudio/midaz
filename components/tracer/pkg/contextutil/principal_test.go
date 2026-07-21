// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package contextutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrincipalContext exercises the WithPrincipal / GetPrincipal round-trip
// matrix. Cases are intentionally narrative — each one pins a load-bearing
// contract documented in principal.go (nil-ctx guard, wrong-type rejection,
// zero-value distinguishability). Failures preserve the case name via t.Run
// so diagnostics stay specific to the broken contract.
func TestPrincipalContext(t *testing.T) {
	t.Parallel()

	// A typed nil variable avoids SA1012 (passing untyped nil as context).
	// The production guard inside WithPrincipal / GetPrincipal is what we are
	// actually exercising.
	var nilCtx context.Context

	type want struct {
		principal Principal
		ok        bool
	}

	tests := []struct {
		name string
		// build returns the context to inspect. Each case is responsible for
		// constructing whatever shape it needs (background, nil, untyped value,
		// stamped principal). Keeping the builder per-case avoids inventing
		// flags like `wrongTypeValue` / `stampedNilCtx` that would only matter
		// to a single case.
		build func() context.Context
		want  want
	}{
		{
			name: "RoundTrip_user_principal_recovered",
			build: func() context.Context {
				return WithPrincipal(
					context.Background(),
					Principal{Type: "user", ID: "sub-123", Name: "alice"},
				)
			},
			want: want{
				principal: Principal{Type: "user", ID: "sub-123", Name: "alice"},
				ok:        true,
			},
		},
		{
			name: "NilCtx_with_principal_uses_background",
			build: func() context.Context {
				// WithPrincipal must normalize a nil ctx to context.Background()
				// so background callers that haven't established a context yet
				// can still stamp identity.
				return WithPrincipal(nilCtx, Principal{Type: "api_key", ID: "tracer-default"})
			},
			want: want{
				principal: Principal{Type: "api_key", ID: "tracer-default"},
				ok:        true,
			},
		},
		{
			name: "NilCtx_no_principal_returns_not_found",
			build: func() context.Context {
				return nilCtx
			},
			want: want{principal: Principal{}, ok: false},
		},
		{
			name: "Background_ctx_no_principal_returns_not_found",
			build: func() context.Context {
				return context.Background()
			},
			want: want{principal: Principal{}, ok: false},
		},
		{
			name: "WrongTypeInCtx_returns_not_found",
			build: func() context.Context {
				// Simulates misuse: someone stored a string under the principal
				// key. GetPrincipal must NOT type-assert blindly.
				return context.WithValue(
					context.Background(),
					ContextKeyPrincipal{},
					"not-a-principal",
				)
			},
			want: want{principal: Principal{}, ok: false},
		},
		{
			name: "ZeroValuePrincipal_is_present_not_absent",
			build: func() context.Context {
				// A zero-valued Principal stamped explicitly is still "present".
				// GetPrincipal must return (zero, true) so the caller can
				// distinguish "no principal at all" (background worker) from
				// "stamped principal whose fields are empty" — which the audit
				// writer should treat as invalid, NOT as a system event.
				return WithPrincipal(context.Background(), Principal{})
			},
			want: want{principal: Principal{}, ok: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := tt.build()
			got, ok := GetPrincipal(ctx)

			if tt.want.ok {
				require.True(t, ok, "expected principal to be present")
			} else {
				require.False(t, ok, "expected principal to be absent")
			}

			assert.Equal(t, tt.want.principal, got)
		})
	}
}
