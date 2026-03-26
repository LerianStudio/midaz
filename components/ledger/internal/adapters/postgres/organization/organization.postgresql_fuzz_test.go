// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package organization

import (
	"context"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// FUZZ TESTS -- getDB Context Tenant PG Connection Resolution
//
// This fuzz test exercises getDB with arbitrary context values alongside
// the generic tenant PG connection via tmcore.ContextWithTenantPGConnection
// to verify:
//  1. No panic under any context content (including Unicode, null bytes, very
//     long strings, empty strings, and security payloads).
//  2. Path selection invariants are preserved: tenant DB present -> returns it,
//     absent -> fallback path.
//  3. Determinism: same input always produces the same path.
//
// Run with:
//
//	go test -run=^$ -fuzz=FuzzGetDB_TenantPGConnection -fuzztime=30s \
//	    ./components/ledger/internal/adapters/postgres/organization/
//
// =============================================================================

// FuzzGetDB_TenantPGConnection fuzzes arbitrary context values to verify
// that getDB never panics regardless of what values are present in the
// request context alongside the tenant PG connection.
//
// Invariants verified for every input:
//   - No panic
//   - When hasTenant is true -> returns the tenant DB (no error)
//   - When hasTenant is false -> takes the fallback path (static connection)
//   - Determinism: two calls with the same context produce the same result
func FuzzGetDB_TenantPGConnection(f *testing.F) {
	// Seed corpus: entries covering various categories.
	// Each entry is (contextValue string, hasTenant bool).

	// 1. With tenant connection
	f.Add("normal", true)
	// 2. Without tenant connection
	f.Add("normal", false)
	// 3. Empty context value
	f.Add("", true)
	// 4. Unicode
	f.Add("\u65e5\u672c\u8a9e\u30c6\u30b9\u30c8", true) // Japanese
	// 5. Security: SQL injection attempt
	f.Add("'; DROP TABLE organization;--", true)
	// 6. Boundary: very long string (256 chars)
	f.Add(strings.Repeat("x", 256), false)
	// 7. Security: null byte injection
	f.Add("value\x00extra", true)

	f.Fuzz(func(t *testing.T, contextValue string, hasTenant bool) {
		// Bound input length to prevent resource exhaustion (OOM).
		if len(contextValue) > 256 {
			contextValue = contextValue[:256]
		}

		// Arrange: create a context with an arbitrary value and optionally
		// inject a tenant DB.
		type ctxKey struct{ name string }
		tenantDB := &mockDB{}

		ctx := context.WithValue(context.Background(), ctxKey{name: "fuzz"}, contextValue)
		if hasTenant {
			ctx = tmcore.ContextWithTenantPGConnection(ctx, tenantDB)
		}

		repo := &OrganizationPostgreSQLRepository{
			connection: newPlaceholderPostgresConnection(),
			tableName:  "organization",
		}

		// Act: call getDB -- must not panic (covered by test execution).
		db, err := repo.getDB(ctx)

		// Invariant 1: when tenant DB is in context, getDB must return it.
		if hasTenant {
			require.NoError(t, err,
				"getDB must not error when context has tenant PG connection")
			require.NotNil(t, db,
				"getDB must return a non-nil DB when tenant PG connection is present")
			assert.Same(t, tenantDB, db,
				"getDB must return the exact tenant DB injected into context")
		}

		// Invariant 2: when no tenant DB is in context, getDB falls back
		// to the static connection. With a placeholder connection (no real
		// PostgreSQL server), this path returns an error.
		if !hasTenant {
			require.Error(t, err,
				"getDB must fall back to static connection when no tenant DB in context")
			assert.Nil(t, db,
				"getDB must return nil DB when static fallback fails")
		}

		// Invariant 3: determinism -- calling getDB again with the same
		// context must produce the same outcome.
		db2, err2 := repo.getDB(ctx)
		if err == nil {
			require.NoError(t, err2,
				"determinism: second call must also succeed")
			assert.Same(t, db, db2,
				"determinism: second call must return the same DB instance")
		} else {
			require.Error(t, err2,
				"determinism: second call must also fail")
			assert.Nil(t, db2,
				"determinism: second call must also return nil DB")
		}
	})
}
