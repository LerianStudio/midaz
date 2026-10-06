// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// migrationsDir returns the absolute path to the migrations directory
// where this test file lives.
func migrationsDir(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller information")

	return filepath.Dir(filename)
}

// readMigration returns the raw text of one migration file.
func readMigration(t *testing.T, name string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(migrationsDir(t), name))
	require.NoError(t, err, "migration file %s must be readable", name)
	require.NotEmpty(t, body, "migration file %s must not be empty", name)

	return string(body)
}

// statementCount counts top-level statements by terminating semicolons after
// dropping `--` comments. It does not understand dollar quoting, so it is only
// meaningful for files that carry no PL/pgSQL body.
func statementCount(sql string) int {
	return strings.Count(stripLineComments(sql), ";")
}

// stripLineComments drops every `--` comment so a word in prose is not taken
// for SQL.
func stripLineComments(sql string) string {
	lines := strings.Split(sql, "\n")

	for i, line := range lines {
		lines[i], _, _ = strings.Cut(line, "--")
	}

	return strings.Join(lines, "\n")
}
