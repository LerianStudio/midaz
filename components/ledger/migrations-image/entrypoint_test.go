// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrationsimage

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubMigrate is a POSIX-sh `migrate` replacement placed on PATH. Instead of
// connecting to a database, it appends one line per invocation to the file named
// by MIGRATE_CAPTURE, recording every argument separated by an ASCII Unit
// Separator (0x1f) so values containing spaces stay intact, then exits 0. This
// lets the test inspect the exact `-path` / `-database` values entrypoint.sh
// assembled without any real database.
const stubMigrate = `#!/bin/sh
line=""
for arg in "$@"; do
	line="${line}${arg}` + "\x1f" + `"
done
printf '%s\n' "$line" >> "$MIGRATE_CAPTURE"
exit 0
`

// invocation is one captured `migrate` call, split into its arguments.
type invocation struct {
	args []string
}

// flag returns the value following the given flag name (e.g. "-database"), and
// whether it was present.
func (in invocation) flag(name string) (string, bool) {
	for i := 0; i < len(in.args)-1; i++ {
		if in.args[i] == name {
			return in.args[i+1], true
		}
	}

	return "", false
}

func TestEntrypoint_DSNAssembly(t *testing.T) {
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
		// assert receives the two captured invocations in call order
		// (onboarding first, transaction second).
		assert func(t *testing.T, calls []invocation)
	}{
		{
			name: "url overrides are used verbatim",
			env: map[string]string{
				"ONBOARDING_DATABASE_URL":  "postgres://verbatim-onb@host:1/onb?sslmode=require",
				"TRANSACTION_DATABASE_URL": "postgres://verbatim-txn@host:2/txn?sslmode=require",
			},
			assert: func(t *testing.T, calls []invocation) {
				onbDB, ok := calls[0].flag("-database")
				require.True(t, ok, "onboarding invocation must carry -database")
				require.Equal(t, "postgres://verbatim-onb@host:1/onb?sslmode=require", onbDB)

				txnDB, ok := calls[1].flag("-database")
				require.True(t, ok, "transaction invocation must carry -database")
				require.Equal(t, "postgres://verbatim-txn@host:2/txn?sslmode=require", txnDB)
			},
		},
		{
			name: "assembled dsn percent-encodes the password",
			env: map[string]string{
				"DB_ONBOARDING_HOST":     "onb-host",
				"DB_ONBOARDING_PORT":     "6001",
				"DB_ONBOARDING_USER":     "onb_user",
				"DB_ONBOARDING_PASSWORD": "p@ss:/% word",
				"DB_ONBOARDING_NAME":     "onb_db",
				"DB_ONBOARDING_SSLMODE":  "require",

				"DB_TRANSACTION_HOST":     "txn-host",
				"DB_TRANSACTION_PORT":     "6002",
				"DB_TRANSACTION_USER":     "txn_user",
				"DB_TRANSACTION_PASSWORD": "p@ss:/% word",
				"DB_TRANSACTION_NAME":     "txn_db",
				"DB_TRANSACTION_SSLMODE":  "require",
			},
			assert: func(t *testing.T, calls []invocation) {
				// `%` is encoded first, so the literal `%` becomes `%25` and is
				// NOT re-touched by later rules (no double-encoding):
				//   @ -> %40  : -> %3A  / -> %2F  % -> %25  space -> %20
				wantPwd := "p%40ss%3A%2F%25%20word"

				onbDB, ok := calls[0].flag("-database")
				require.True(t, ok, "onboarding invocation must carry -database")
				require.Equal(t,
					"postgres://onb_user:"+wantPwd+"@onb-host:6001/onb_db?sslmode=require",
					onbDB,
				)

				txnDB, ok := calls[1].flag("-database")
				require.True(t, ok, "transaction invocation must carry -database")
				require.Equal(t,
					"postgres://txn_user:"+wantPwd+"@txn-host:6002/txn_db?sslmode=require",
					txnDB,
				)
			},
		},
		{
			name: "port and sslmode default when unset",
			env: map[string]string{
				"DB_ONBOARDING_HOST":     "onb-host",
				"DB_ONBOARDING_USER":     "onb_user",
				"DB_ONBOARDING_PASSWORD": "simple",
				"DB_ONBOARDING_NAME":     "onb_db",

				"DB_TRANSACTION_HOST":     "txn-host",
				"DB_TRANSACTION_USER":     "txn_user",
				"DB_TRANSACTION_PASSWORD": "simple",
				"DB_TRANSACTION_NAME":     "txn_db",
			},
			assert: func(t *testing.T, calls []invocation) {
				onbDB, ok := calls[0].flag("-database")
				require.True(t, ok, "onboarding invocation must carry -database")
				require.Equal(t,
					"postgres://onb_user:simple@onb-host:5432/onb_db?sslmode=disable",
					onbDB,
				)

				txnDB, ok := calls[1].flag("-database")
				require.True(t, ok, "transaction invocation must carry -database")
				require.Equal(t,
					"postgres://txn_user:simple@txn-host:5432/txn_db?sslmode=disable",
					txnDB,
				)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := runEntrypoint(t, entrypoint, tt.env)

			// Ordering + path contract holds for every case: onboarding first,
			// then transaction.
			require.Len(t, calls, 2, "entrypoint must invoke migrate exactly twice")

			onbPath, ok := calls[0].flag("-path")
			require.True(t, ok, "first invocation must carry -path")
			require.Equal(t, "/migrations/onboarding", onbPath,
				"first migrate call must target onboarding")

			txnPath, ok := calls[1].flag("-path")
			require.True(t, ok, "second invocation must carry -path")
			require.Equal(t, "/migrations/transaction", txnPath,
				"second migrate call must target transaction")

			tt.assert(t, calls)
		})
	}
}

// runEntrypoint executes entrypoint.sh with a stub `migrate` on PATH and returns
// the captured invocations in call order.
func runEntrypoint(t *testing.T, entrypoint string, env map[string]string) []invocation {
	t.Helper()

	binDir := t.TempDir()

	stubPath := filepath.Join(binDir, "migrate")
	require.NoError(t, os.WriteFile(stubPath, []byte(stubMigrate), 0o755),
		"failed to write stub migrate")

	capturePath := filepath.Join(t.TempDir(), "capture.txt")

	cmd := exec.Command("sh", entrypoint)
	cmd.Env = append([]string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"MIGRATE_CAPTURE=" + capturePath,
	}, envSlice(env)...)

	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "entrypoint.sh failed: %v\noutput:\n%s", err, out)

	// The entrypoint must not leak any secret into stdout/stderr.
	require.NotContains(t, string(out), "p@ss", "entrypoint output must not echo passwords")

	return readCapture(t, capturePath)
}

// envSlice renders a map into KEY=VALUE entries for exec.Cmd.Env.
func envSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}

	return out
}

// readCapture parses the stub's capture file into ordered invocations.
func readCapture(t *testing.T, path string) []invocation {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read capture file")

	var calls []invocation

	for line := range strings.SplitSeq(strings.TrimRight(string(raw), "\n"), "\n") {
		if line == "" {
			continue
		}

		// Each field is terminated by the Unit Separator; drop the trailing empty.
		fields := strings.Split(line, "\x1f")
		if n := len(fields); n > 0 && fields[n-1] == "" {
			fields = fields[:n-1]
		}

		calls = append(calls, invocation{args: fields})
	}

	return calls
}
