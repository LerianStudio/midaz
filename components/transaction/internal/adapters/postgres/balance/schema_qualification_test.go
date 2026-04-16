// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance_test

// Context — why this test exists:
// Migration 000022/000023 (staged_cutover) renames "operation" → "operation_legacy"
// and "operation_partitioned" → "operation"; the same shape applies to
// "balance". After the cutover, an unqualified identifier in a SELECT/UPDATE/
// INSERT/DELETE resolves via the session's search_path. That is a silent
// correctness regression vector: a misconfigured tenant search_path, an
// extension that pushes a schema onto the path, or a future "operation" table
// created in another schema would all silently redirect reads/writes AWAY
// from public.operation — with no SQL error to flag the drift.
//
// The defensive fix is to schema-qualify every "operation" and "balance"
// reference inside the transaction-service postgres adapters with "public.".
// This test walks the adapter tree (balance/ and operation/ packages, which
// are the only owners of the renamed tables) and fails the build if any
// naked reference slips through on a future PR.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These patterns catch FROM / INTO / UPDATE / JOIN clauses that touch
// "operation" or "balance" as an isolated identifier. The trailing class
// (\s|\)|end-of-line/backtick) ensures we only flag standalone words — so
// "operation_route", "balance_legacy", "operation_transaction_route", etc.,
// are correctly ignored.
//
// The patterns are CASE-SENSITIVE on purpose: real SQL in this repo uses
// UPPERCASE keywords, so matching `UPDATE balance` catches queries while
// leaving English prose ("Failed to update balance from redis") in log
// strings untouched. We kept the natural-language log messages audit-
// friendly instead of escaping them into technical jargon.
var unqualifiedBalanceOrOperationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(FROM|UPDATE|JOIN)\s+(operation|balance)(\s|\)|` + "`" + `|$)`),
	regexp.MustCompile(`\bINSERT\s+INTO\s+(operation|balance)(\s|\()`),
	regexp.MustCompile(`\bDELETE\s+FROM\s+(operation|balance)(\s|\)|` + "`" + `|$)`),
	// Squirrel builder literals: .From("operation"), .From("balance b"),
	// .Insert("balance"), .Update("operation"). We match the literal so
	// higher-level builder usage stays in scope too.
	regexp.MustCompile(`\.(From|Insert|Update|Delete)\("(operation|balance)(\s[^"]*)?"\)`),
	// tableName: "operation" / "balance" without public. qualifier. This is
	// the primary constructor assignment guarded by the fix — if someone
	// unwinds it, the test fires immediately.
	regexp.MustCompile(`tableName:\s+"(operation|balance)"`),
}

// TestRepository_QueriesUseSchemaQualifiedTableNames is the ripple-effect
// regression for the table-rename cutover. Scoped to the two packages that
// own the renamed tables (balance/ + operation/). Test files are excluded —
// integration tests own their schema via testcontainers and can keep literal
// fixtures. dualwrite.go is excluded because it intentionally writes to the
// partitioned shell tables while the cutover phase is active.
func TestRepository_QueriesUseSchemaQualifiedTableNames(t *testing.T) {
	// Resolve the two packages that own "balance" and "operation" tables.
	// This test runs from components/.../postgres/balance, so the balance
	// sources are "." and the operation sources are "../operation".
	cwd, err := os.Getwd()
	require.NoError(t, err)

	scanRoots := []string{cwd, filepath.Join(cwd, "..", "operation")}

	var findings []string

	for _, root := range scanRoots {
		if _, statErr := os.Stat(root); os.IsNotExist(statErr) {
			// Skip cleanly if the companion package was refactored away —
			// a future rename to pkg/mmodel/operation should not break
			// this regression.
			continue
		}

		walkErr := filepath.Walk(root, func(path string, info os.FileInfo, werr error) error {
			if werr != nil {
				return werr
			}

			if info.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".go") {
				return nil
			}

			if strings.HasSuffix(path, "_test.go") {
				return nil
			}

			if strings.HasSuffix(path, "/dualwrite.go") {
				return nil
			}

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("read %s: %w", path, readErr)
			}

			for _, re := range unqualifiedBalanceOrOperationPatterns {
				for _, match := range re.FindAllIndex(data, -1) {
					// Precise line-number diagnostic.
					line := 1 + strings.Count(string(data[:match[0]]), "\n")

					findings = append(findings,
						relOrPath(root, path)+":"+strconv.Itoa(line)+": "+string(data[match[0]:match[1]]))
				}
			}

			return nil
		})
		require.NoError(t, walkErr)
	}

	assert.Empty(t, findings,
		"found unqualified references to renamed tables (operation/balance); "+
			"post-cutover these resolve via search_path and can silently redirect reads/writes. "+
			"Replace with `public.operation` / `public.balance` (or set r.tableName = \"public.<name>\"). "+
			"See migrations/staged_cutover/000022 and 000023 for the rename rationale.\n"+
			"Findings:\n"+strings.Join(findings, "\n"))
}

// relOrPath returns a compact path relative to root when possible, falling
// back to the absolute path so the failure message stays actionable on both
// local runs and CI (which may use different CWDs).
func relOrPath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return rel
	}

	return path
}
