// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrationsimage

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	migrateentrypoint "github.com/LerianStudio/midaz/v4/tests/utils/migrateentrypoint"
)

func TestEntrypoint_DSNAssembly(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available on PATH; skipping shell entrypoint test")
	}

	entrypoint, err := filepath.Abs("entrypoint.sh")
	require.NoError(t, err, "failed to resolve entrypoint.sh path")

	if _, err := os.Stat(entrypoint); err != nil {
		t.Skipf("entrypoint.sh not found at %s: %v", entrypoint, err)
	}

	tests := []struct {
		name string
		env  map[string]string
		// assert receives the single captured invocation (tracer is single-DB).
		assert func(t *testing.T, call migrateentrypoint.Invocation)
	}{
		{
			name: "url override is used verbatim",
			env: map[string]string{
				"DATABASE_URL": "postgres://verbatim-tracer@host:1/tracer?sslmode=require",
			},
			assert: func(t *testing.T, call migrateentrypoint.Invocation) {
				db, ok := call.Flag("-database")
				require.True(t, ok, "invocation must carry -database")
				require.Equal(t, "postgres://verbatim-tracer@host:1/tracer?sslmode=require", db)
				// The override is passed through verbatim: encode_password is
				// bypassed entirely, so the literal `@` is NOT percent-encoded.
				require.NotContains(t, db, "%40",
					"override DSN must not be percent-encoded")
			},
		},
		{
			name: "assembled dsn percent-encodes the password",
			env: map[string]string{
				"DB_HOST":     "trc-host",
				"DB_PORT":     "6001",
				"DB_USER":     "trc_user",
				"DB_PASSWORD": "p@ss:/% word?#&+[]",
				"DB_NAME":     "trc_db",
				"DB_SSL_MODE": "require",
			},
			assert: func(t *testing.T, call migrateentrypoint.Invocation) {
				// encode_password substitutes `%` FIRST so already-inserted
				// escapes are not double-encoded, then the remaining reserved
				// characters in order:
				//   % -> %25  @ -> %40  : -> %3A  / -> %2F  ? -> %3F
				//   # -> %23  & -> %26  + -> %2B  space -> %20
				//   [ -> %5B  ] -> %5D
				// All 11 reserved characters the sed chain handles are covered.
				wantPwd := "p%40ss%3A%2F%25%20word%3F%23%26%2B%5B%5D"

				db, ok := call.Flag("-database")
				require.True(t, ok, "invocation must carry -database")
				require.Equal(t,
					"postgres://trc_user:"+wantPwd+"@trc-host:6001/trc_db?sslmode=require",
					db,
				)
			},
		},
		{
			name: "port and sslmode default when unset",
			env: map[string]string{
				"DB_HOST":     "trc-host",
				"DB_USER":     "trc_user",
				"DB_PASSWORD": "simple",
				"DB_NAME":     "trc_db",
			},
			assert: func(t *testing.T, call migrateentrypoint.Invocation) {
				db, ok := call.Flag("-database")
				require.True(t, ok, "invocation must carry -database")
				require.Equal(t,
					"postgres://trc_user:simple@trc-host:5432/trc_db?sslmode=disable",
					db,
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			calls := migrateentrypoint.RunEntrypoint(t, entrypoint, tt.env)

			// Single-DB contract: the tracer runner invokes migrate exactly once,
			// always targeting the bundled /migrations path.
			require.Len(t, calls, 1, "entrypoint must invoke migrate exactly once")

			path, ok := calls[0].Flag("-path")
			require.True(t, ok, "invocation must carry -path")
			require.Equal(t, "/migrations", path,
				"migrate call must target the bundled tracer migrations path")

			up := calls[0].Args[len(calls[0].Args)-1]
			require.Equal(t, "up", up, "migrate call must be an `up` invocation")

			tt.assert(t, calls[0])
		})
	}
}
