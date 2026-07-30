// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
)

// TestAcquireRead_RequireTenantFailsClosed locks the read-seam invariant: with
// requireTenant=true and routeTxReadsToPrimary=true, a routed read (primary-read
// intent marked in ctx) that carries NO tenant postgres handle MUST fail closed
// with the explicit tenant-missing error BEFORE any static-fallback / routing
// decision. The repository is built with a nil static connection so ANY static
// fallback would surface a distinct failure ("postgres connection not available"
// or a nil-conn panic) rather than the tenant-missing sentinel — proving getDB's
// tenant guard fires FIRST (check order), and acquireRead never reaches the
// routing/tx path on a wrong handle.
func TestAcquireRead_RequireTenantFailsClosed(t *testing.T) {
	const wantErr = "tenant postgres connection missing from context"

	// A single non-empty entry so the read method does not short-circuit on the
	// empty-input fast path and actually reaches acquireRead -> getDB.
	aliasesWithKeys := []string{"alias1#default"}
	orgID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name string
		call func(r *BalancePostgreSQLRepository, ctx context.Context) error
	}{
		{
			name: "ListByAliasesWithKeys",
			call: func(r *BalancePostgreSQLRepository, ctx context.Context) error {
				_, err := r.ListByAliasesWithKeys(ctx, orgID, ledgerID, aliasesWithKeys)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// connection=nil => any static fallback would fail distinctly.
			// routeTxReadsToPrimary=true, requireTenant=true.
			repo := NewBalancePostgreSQLRepository(nil, true, true)

			// MARKED primary-read intent, but NO tenant handle injected into ctx.
			ctx := readrouting.WithPrimaryRead(context.Background())

			// Guard: the intent is actually marked, so this exercises the routed
			// path, not a plain read.
			if !readrouting.IsPrimaryRead(ctx) {
				t.Fatalf("test setup: expected primary-read intent to be marked in ctx")
			}

			err := tt.call(repo, ctx)

			if err == nil {
				t.Fatalf("expected tenant-missing error, got nil (read must fail closed, not fall back to static connection)")
			}

			// Assert on the SPECIFIC tenant-missing message, not a generic non-nil
			// error. A different message (e.g. "postgres connection not available")
			// would mean a static-fallback attempt slipped past the tenant guard.
			if got := err.Error(); got != wantErr {
				t.Fatalf("expected tenant-missing error %q, got %q — the tenant guard must fire before any static fallback or routing decision", wantErr, got)
			}
		})
	}
}
