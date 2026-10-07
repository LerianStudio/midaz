// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package migrations

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hashesManifest = ".hashes"

// TestHashesManifestMatchesMigrationFiles locks the integrity manifest to the
// migration files on disk: every entry must match its file byte for byte, and
// every .sql file (top level and seeds/) must have an entry. A drift on either
// side means the manifest was not regenerated with the command in its header.
func TestHashesManifestMatchesMigrationFiles(t *testing.T) {
	t.Parallel()

	dir := migrationsDir(t)

	manifest, err := os.Open(filepath.Join(dir, hashesManifest))
	require.NoError(t, err, "%s must exist beside the migrations", hashesManifest)

	defer func() { require.NoError(t, manifest.Close()) }()

	listed := make(map[string]string)
	scanner := bufio.NewScanner(manifest)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		require.Len(t, fields, 2, "manifest line %q must be '<sha256>  <path>'", line)
		require.Len(t, fields[0], sha256.Size*2, "manifest line %q must carry a hex sha256", line)

		_, duplicate := listed[fields[1]]
		require.False(t, duplicate, "manifest lists %s twice", fields[1])

		listed[fields[1]] = fields[0]
	}

	require.NoError(t, scanner.Err())

	onDisk := migrationSQLFiles(t, dir)

	for _, rel := range onDisk {
		want, ok := listed[rel]
		if !assert.True(t, ok, "%s has no entry in %s", rel, hashesManifest) {
			continue
		}

		body, err := os.ReadFile(filepath.Join(dir, rel))
		require.NoError(t, err)

		sum := sha256.Sum256(body)
		assert.Equal(t, want, hex.EncodeToString(sum[:]), "%s does not match its %s entry", rel, hashesManifest)
	}

	for rel := range listed {
		_, err := os.Stat(filepath.Join(dir, rel))
		assert.NoError(t, err, "%s lists %s, which is not on disk", hashesManifest, rel)
	}
}

// migrationSQLFiles returns the manifest-relative paths of every migration
// and seed SQL file.
func migrationSQLFiles(t *testing.T, dir string) []string {
	t.Helper()

	var files []string

	for _, pattern := range []string{"*.sql", filepath.Join("seeds", "*.sql")} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		require.NoError(t, err)

		for _, match := range matches {
			rel, err := filepath.Rel(dir, match)
			require.NoError(t, err)

			files = append(files, filepath.ToSlash(rel))
		}
	}

	require.NotEmpty(t, files, "no migration SQL files found under %s", dir)

	return files
}
