// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package migrateentrypoint provides a shared test harness for exercising the
// migration-runner image entrypoint.sh scripts (ledger and tracer). It runs the
// shell entrypoint with a stubbed `migrate` on PATH and captures the exact
// arguments each `migrate` invocation received, so per-service tests can assert
// the assembled `-path` / `-database` values without a real database.
package migrateentrypoint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// StubMigrate is a POSIX-sh `migrate` replacement placed on PATH. Instead of
// connecting to a database, it appends one line per invocation to the file named
// by MIGRATE_CAPTURE, recording every argument separated by an ASCII Unit
// Separator (0x1f) so values containing spaces stay intact, then exits 0. This
// lets tests inspect the exact `-path` / `-database` values entrypoint.sh
// assembled without any real database.
const StubMigrate = `#!/bin/sh
line=""
for arg in "$@"; do
	line="${line}${arg}` + "\x1f" + `"
done
printf '%s\n' "$line" >> "$MIGRATE_CAPTURE"
exit 0
`

// Invocation is one captured `migrate` call, split into its arguments.
type Invocation struct {
	Args []string
}

// Flag returns the value following the given flag name (e.g. "-database"), and
// whether it was present.
func (in Invocation) Flag(name string) (string, bool) {
	for i := 0; i < len(in.Args)-1; i++ {
		if in.Args[i] == name {
			return in.Args[i+1], true
		}
	}

	return "", false
}

// RunEntrypoint executes entrypoint.sh with a stub `migrate` on PATH and returns
// the captured invocations in call order. It also asserts the entrypoint never
// echoes the sentinel password fragment ("p@ss") to stdout/stderr.
//
// It is parallel-safe: each call gets its own t.TempDir() for the stub and the
// capture file, and environment is applied only to the child exec.Cmd — never to
// the parent process.
func RunEntrypoint(t *testing.T, entrypoint string, env map[string]string) []Invocation {
	t.Helper()

	binDir := t.TempDir()

	stubPath := filepath.Join(binDir, "migrate")
	require.NoError(t, os.WriteFile(stubPath, []byte(StubMigrate), 0o755),
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
func readCapture(t *testing.T, path string) []Invocation {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read capture file")

	var calls []Invocation

	for line := range strings.SplitSeq(strings.TrimRight(string(raw), "\n"), "\n") {
		if line == "" {
			continue
		}

		// Each field is terminated by the Unit Separator; drop the trailing empty.
		fields := strings.Split(line, "\x1f")
		if n := len(fields); n > 0 && fields[n-1] == "" {
			fields = fields[:n-1]
		}

		calls = append(calls, Invocation{Args: fields})
	}

	return calls
}
