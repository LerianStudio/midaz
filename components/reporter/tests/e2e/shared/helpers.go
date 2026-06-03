// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package shared

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// UniqueID generates a unique identifier with the given prefix for test isolation.
func UniqueID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)

	return prefix + "-" + hex.EncodeToString(b)
}

// LoadFixture reads a fixture file from the testdata/ directory.
func LoadFixture(t *testing.T, path string) []byte {
	t.Helper()

	fullPath := filepath.Join(testdataDir(), path)

	data, err := os.ReadFile(fullPath)
	require.NoError(t, err, "load fixture %s", path)

	return data
}

// testdataDir returns the absolute path to the testdata directory.
func testdataDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}

	return filepath.Join(filepath.Dir(file), "..", "testdata")
}

// FilterCondition defines advanced filtering conditions for report generation.
// Mirrors pkg/model.FilterCondition but lives in the test package for independence.
type FilterCondition struct {
	Equals         []any `json:"eq,omitempty"`
	GreaterThan    []any `json:"gt,omitempty"`
	GreaterOrEqual []any `json:"gte,omitempty"`
	LessThan       []any `json:"lt,omitempty"`
	LessOrEqual    []any `json:"lte,omitempty"`
	Between        []any `json:"between,omitempty"`
	In             []any `json:"in,omitempty"`
	NotIn          []any `json:"nin,omitempty"`
}

// FilterEq creates a FilterCondition with Equals operator.
func FilterEq(values ...any) FilterCondition {
	return FilterCondition{Equals: values}
}

// FilterGt creates a FilterCondition with GreaterThan operator.
func FilterGt(value any) FilterCondition {
	return FilterCondition{GreaterThan: []any{value}}
}

// FilterGte creates a FilterCondition with GreaterOrEqual operator.
func FilterGte(value any) FilterCondition {
	return FilterCondition{GreaterOrEqual: []any{value}}
}

// FilterLt creates a FilterCondition with LessThan operator.
func FilterLt(value any) FilterCondition {
	return FilterCondition{LessThan: []any{value}}
}

// FilterLte creates a FilterCondition with LessOrEqual operator.
func FilterLte(value any) FilterCondition {
	return FilterCondition{LessOrEqual: []any{value}}
}

// FilterBetween creates a FilterCondition with Between operator.
func FilterBetween(min, max any) FilterCondition {
	return FilterCondition{Between: []any{min, max}}
}

// FilterIn creates a FilterCondition with In operator.
func FilterIn(values ...any) FilterCondition {
	return FilterCondition{In: values}
}

// FilterNotIn creates a FilterCondition with NotIn operator.
func FilterNotIn(values ...any) FilterCondition {
	return FilterCondition{NotIn: values}
}

// SaveReport persists downloaded report data to disk for manual inspection.
// Only active when E2E_SAVE_REPORTS=true. Files are saved to E2E_REPORTS_DIR
// (default: /tmp/e2e-reports/) with the pattern <testName>.<ext>.
// The function logs the output path and never fails the test on write errors.
func SaveReport(t *testing.T, data []byte, format string) {
	t.Helper()

	if !strings.EqualFold(os.Getenv("E2E_SAVE_REPORTS"), "true") {
		return
	}

	dir := os.Getenv("E2E_REPORTS_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "e2e-reports")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("[save-report] failed to create dir %s: %v", dir, err)
		return
	}

	ext := formatExtension(format)
	filename := sanitizeTestName(t.Name()) + ext
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Logf("[save-report] failed to write %s: %v", path, err)
		return
	}

	t.Logf("[save-report] saved %d bytes → %s", len(data), path)
}

// formatExtension maps output format strings to file extensions.
func formatExtension(format string) string {
	switch strings.ToLower(format) {
	case FormatHTML:
		return ".html"
	case FormatCSV:
		return ".csv"
	case FormatXML:
		return ".xml"
	case FormatPDF:
		return ".pdf"
	case FormatTXT:
		return ".txt"
	default:
		return ".bin"
	}
}

// sanitizeTestName replaces characters that are invalid in filenames.
func sanitizeTestName(name string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", " ", "_")
	return r.Replace(name)
}

// MakeFilters creates a filter map for a single datasource/table/field condition.
func MakeFilters(datasource, table, field string, condition FilterCondition) map[string]map[string]map[string]FilterCondition {
	return map[string]map[string]map[string]FilterCondition{
		datasource: {
			table: {
				field: condition,
			},
		},
	}
}
