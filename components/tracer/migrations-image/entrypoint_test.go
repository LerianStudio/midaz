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
		// assert receives the single captured invocation (tracer is single-DB).
		assert func(t *testing.T, call invocation)
	}{
		{
			name: "url override is used verbatim",
			env: map[string]string{
				"DATABASE_URL": "postgres://verbatim-tracer@host:1/tracer?sslmode=require",
			},
			assert: func(t *testing.T, call invocation) {
				db, ok := call.flag("-database")
				require.True(t, ok, "invocation must carry -database")
				require.Equal(t, "postgres://verbatim-tracer@host:1/tracer?sslmode=require", db)
			},
		},
		{
			name: "assembled dsn percent-encodes the password",
			env: map[string]string{
				"DB_HOST":     "trc-host",
				"DB_PORT":     "6001",
				"DB_USER":     "trc_user",
				"DB_PASSWORD": "p@ss:/% word",
				"DB_NAME":     "trc_db",
				"DB_SSL_MODE": "require",
			},
			assert: func(t *testing.T, call invocation) {
				// `%` is encoded first, so the literal `%` becomes `%25` and is
				// NOT re-touched by later rules (no double-encoding):
				//   @ -> %40  : -> %3A  / -> %2F  % -> %25  space -> %20
				wantPwd := "p%40ss%3A%2F%25%20word"

				db, ok := call.flag("-database")
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
			assert: func(t *testing.T, call invocation) {
				db, ok := call.flag("-database")
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
			calls := runEntrypoint(t, entrypoint, tt.env)

			// Single-DB contract: the tracer runner invokes migrate exactly once,
			// always targeting the bundled /migrations path.
			require.Len(t, calls, 1, "entrypoint must invoke migrate exactly once")

			path, ok := calls[0].flag("-path")
			require.True(t, ok, "invocation must carry -path")
			require.Equal(t, "/migrations", path,
				"migrate call must target the bundled tracer migrations path")

			up := calls[0].args[len(calls[0].args)-1]
			require.Equal(t, "up", up, "migrate call must be an `up` invocation")

			tt.assert(t, calls[0])
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

	// CombinedOutput runs the command to completion and reaps it, so no child
	// process or pipe is left dangling.
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
