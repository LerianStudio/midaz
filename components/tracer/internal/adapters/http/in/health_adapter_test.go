// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresConnectionAdapter_NilSafety locks in the contract that both
// IsConnected and GetDB tolerate a typed-nil receiver and a nil inner
// connection. Without these guards a half-constructed *HealthChecker — for
// example one whose dbProvider was never wired because libPostgres.Client
// initialisation failed — would panic on the readiness probe, and kubelet
// would interpret the panic as a hard failure and kill the pod, masking the
// real configuration bug behind a restart loop.
//
// IsConnected was the gap CodeRabbit flagged on PR #151: GetDB already
// short-circuited on nil receiver/conn, but IsConnected dereferenced p.conn
// directly and panicked when called on a typed-nil PostgresDBProvider.
func TestPostgresConnectionAdapter_NilSafety(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		adapter   *postgresConnectionAdapter
		wantReady bool
	}{
		{
			name:      "Nil receiver - IsConnected returns false instead of panicking",
			adapter:   nil,
			wantReady: false,
		},
		{
			name: "Nil inner conn - IsConnected returns false instead of panicking",
			// Adapter struct exists but its conn was never wired; can happen
			// when libPostgres.Client construction failed and the caller
			// forgot to short-circuit.
			adapter:   &postgresConnectionAdapter{},
			wantReady: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// IsConnected must not panic and must report "not connected".
			require.NotPanics(t, func() {
				got := tt.adapter.IsConnected()
				assert.Equal(t, tt.wantReady, got,
					"nil-state adapter must report IsConnected=false")
			})

			// GetDB must mirror the same nil-safety: surface a clean
			// sentinel instead of crashing. The exact error is a
			// pre-existing contract verified by other tests; here we just
			// assert the absence of a panic + that an error is returned.
			require.NotPanics(t, func() {
				db, err := tt.adapter.GetDB(context.Background())
				assert.Nil(t, db, "nil-state adapter must not return a *sql.DB")
				require.Error(t, err, "nil-state adapter must surface a sentinel error")
				assert.ErrorIs(t, err, ErrConnectionNotEstablished,
					"nil-state adapter must return the registered ErrConnectionNotEstablished sentinel")
			})
		})
	}
}
