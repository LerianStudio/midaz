// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

// =============================================================================
// FUZZ TESTS -- buildPostgresConnection Config Values
//
// This fuzz test exercises buildPostgresConnection with arbitrary Config field
// values to verify:
//   1. No panic under any combination of field values (including Unicode, null
//      bytes, very long strings, empty strings, and security payloads).
//   2. Always returns a non-nil *PostgresConnection.
//   3. Connection string fields are deterministic: same input -> same output.
//
// Run with:
//
//	go test -run='^$' -fuzz=FuzzBuildPostgresConnection -fuzztime=30s \
//	    ./components/transaction/internal/bootstrap/
//
// =============================================================================

import (
	"strings"
	"testing"

	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FuzzBuildPostgresConnection_ConfigValues fuzzes the 6 core config fields
// (host, port, user, password, dbname, sslmode) to verify that
// buildPostgresConnection never panics regardless of input values.
//
// Invariants verified for every input combination:
//   - No panic (covered by test execution)
//   - Non-nil return value
//   - Determinism: two calls with the same config produce the same result
func FuzzBuildPostgresConnection_ConfigValues(f *testing.F) {
	// Seed corpus: 8+ entries covering the 5 required categories.
	// Fields: host, port, user, password, dbname, sslmode

	// 1. Valid: typical production config
	f.Add("localhost", "5432", "midaz_user", "s3cureP@ss!", "midaz_transaction", "disable")
	// 2. Empty: all empty strings
	f.Add("", "", "", "", "", "")
	// 3. Unicode: non-ASCII characters in all fields
	f.Add("\u65e5\u672c\u8a9e", "\u0041\u0042", "\u00fc\u00e9\u00e8", "\u00df\u00f1", "\u4e2d\u6587", "\u043c\u043e\u0434")
	// 4. Very long strings: 512 chars per field
	f.Add(strings.Repeat("h", 512), strings.Repeat("9", 512), strings.Repeat("u", 512),
		strings.Repeat("p", 512), strings.Repeat("d", 512), strings.Repeat("s", 512))
	// 5. SQL injection: classic injection payloads
	f.Add("localhost'; DROP TABLE transactions;--", "5432", "admin'--", "' OR '1'='1", "midaz'; DELETE FROM users;--", "disable")
	// 6. Null bytes: embedded null characters
	f.Add("host\x00evil", "54\x0032", "user\x00", "\x00pass", "db\x00name", "dis\x00able")
	// 7. Special chars in password: common password special characters
	f.Add("127.0.0.1", "5432", "user", "p@$$w0rd!#%&*(){}[]|\\:\";<>,.?/~`", "mydb", "require")
	// 8. Boundary: spaces, tabs, newlines
	f.Add("  host  ", "\t5432\n", " user ", "pass\nword", "  db  ", " ssl\tmode ")

	logger, err := libZap.InitializeLoggerWithError()
	if err != nil {
		f.Fatalf("failed to initialize logger: %v", err)
	}

	f.Fuzz(func(t *testing.T, host, port, user, password, dbname, sslmode string) {
		// Bound input lengths to prevent resource exhaustion (OOM).
		const maxLen = 512
		if len(host) > maxLen {
			host = host[:maxLen]
		}

		if len(port) > maxLen {
			port = port[:maxLen]
		}

		if len(user) > maxLen {
			user = user[:maxLen]
		}

		if len(password) > maxLen {
			password = password[:maxLen]
		}

		if len(dbname) > maxLen {
			dbname = dbname[:maxLen]
		}

		if len(sslmode) > maxLen {
			sslmode = sslmode[:maxLen]
		}

		cfg := &Config{
			PrimaryDBHost:     host,
			PrimaryDBPort:     port,
			PrimaryDBUser:     user,
			PrimaryDBPassword: password,
			PrimaryDBName:     dbname,
			PrimaryDBSSLMode:  sslmode,
			// Replica fields use same values to exercise both paths.
			ReplicaDBHost:     host,
			ReplicaDBPort:     port,
			ReplicaDBUser:     user,
			ReplicaDBPassword: password,
			ReplicaDBName:     dbname,
			ReplicaDBSSLMode:  sslmode,
		}

		// Act: call buildPostgresConnection -- must not panic (covered by test execution).
		conn := buildPostgresConnection(cfg, logger)

		// Invariant 1: always returns a non-nil connection.
		require.NotNil(t, conn, "buildPostgresConnection must never return nil")

		// Invariant 2: connection string fields are populated (not empty struct).
		assert.NotEmpty(t, conn.ConnectionStringPrimary,
			"ConnectionStringPrimary must not be empty")
		assert.NotEmpty(t, conn.ConnectionStringReplica,
			"ConnectionStringReplica must not be empty")
		assert.Equal(t, ApplicationName, conn.Component,
			"Component must always be ApplicationName")

		// Invariant 3: determinism -- calling again with the same config must
		// produce the same connection strings.
		conn2 := buildPostgresConnection(cfg, logger)
		assert.Equal(t, conn.ConnectionStringPrimary, conn2.ConnectionStringPrimary,
			"determinism: ConnectionStringPrimary must be identical on repeat call")
		assert.Equal(t, conn.ConnectionStringReplica, conn2.ConnectionStringReplica,
			"determinism: ConnectionStringReplica must be identical on repeat call")
	})
}
