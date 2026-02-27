// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// FUZZ TESTS -- getDB Context Module Name Resolution (T-002)
//
// This fuzz test exercises getDB with arbitrary module names injected into
// context via tmcore.ContextWithModulePGConnection to verify:
//   1. No panic under any module name (including Unicode, null bytes, very
//      long strings, empty strings, and security payloads).
//   2. Path selection invariants are preserved: "transaction" -> tenant DB,
//      anything else -> fallback path.
//   3. Determinism: same input always produces the same path.
//
// Run with:
//
//	go test -run=^$ -fuzz=FuzzGetDB_ContextModuleName -fuzztime=30s \
//	    ./components/transaction/internal/adapters/postgres/transaction/
//
// =============================================================================

// FuzzGetDB_ContextModuleName fuzzes the module name stored in context to verify
// that getDB never panics regardless of what module names are present in the
// request context.
//
// Invariants verified for every moduleName:
//   - No panic
//   - moduleName == "transaction" -> returns the tenant DB (no error)
//   - moduleName != "transaction" -> takes the fallback path (static connection)
//   - Determinism: two calls with the same context produce the same result
func FuzzGetDB_ContextModuleName(f *testing.F) {
	// Seed corpus: 8 entries covering the 5 required categories.
	// Each entry is a module name string.

	// 1. Valid: the exact module name used by getDB
	f.Add("transaction")
	// 2. Empty: empty string edge case
	f.Add("")
	// 3. Boundary: different valid module name (miss path)
	f.Add("onboarding")
	// 4. Unicode: non-ASCII characters
	f.Add("\u65e5\u672c\u8a9e\u30c6\u30b9\u30c8") // Japanese
	// 5. Security: SQL injection attempt
	f.Add("transaction'; DROP TABLE transaction;--")
	// 6. Boundary: very long string (256 chars)
	f.Add(strings.Repeat("x", 256))
	// 7. Security: null byte injection
	f.Add("transaction\x00extra")
	// 8. Boundary: case variation (should NOT match "transaction")
	f.Add("TRANSACTION")

	f.Fuzz(func(t *testing.T, moduleName string) {
		// Bound input length to prevent resource exhaustion (OOM).
		if len(moduleName) > 256 {
			moduleName = moduleName[:256]
		}

		// Arrange: inject a mockDB under the fuzz-generated module name
		// and create a repo with a placeholder static connection.
		tenantDB := &mockDB{}
		ctx := tmcore.ContextWithModulePGConnection(context.Background(), moduleName, tenantDB)

		repo := &TransactionPostgreSQLRepository{
			connection: newPlaceholderPostgresConnection(),
			tableName:  "transaction",
		}

		// Act: call getDB -- must not panic (covered by test execution).
		db, err := repo.getDB(ctx)

		// Invariant 1: when moduleName is exactly "transaction", getDB must
		// return the tenant DB from context without error.
		if moduleName == "transaction" {
			require.NoError(t, err,
				"getDB must not error when context has 'transaction' module DB")
			require.NotNil(t, db,
				"getDB must return a non-nil DB for 'transaction' module")
			assert.Same(t, tenantDB, db,
				"getDB must return the exact tenant DB injected into context")
		}

		// Invariant 2: when moduleName is NOT "transaction", getDB falls back
		// to the static connection. With a placeholder connection (no real
		// PostgreSQL server), this path returns an error.
		if moduleName != "transaction" {
			// The fallback path was taken. With a placeholder connection,
			// r.connection.GetDB() returns an error because no DB pools exist.
			require.Error(t, err,
				"getDB must fall back to static connection for module %q", moduleName)
			assert.Nil(t, db,
				"getDB must return nil DB when static fallback fails for module %q", moduleName)
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
