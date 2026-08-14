// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"testing"
	"time"
)

// TestDocumentation_RFC3339Format ensures the documented format examples are valid.
func TestDocumentation_RFC3339Format(t *testing.T) {
	examples := []string{
		"2026-01-28T10:30:00Z",
		"2026-01-28T10:30:00-03:00",
		"2024-01-15T00:00:00Z",
	}

	for _, example := range examples {
		t.Run(example, func(t *testing.T) {
			_, err := time.Parse(time.RFC3339, example)
			if err != nil {
				t.Errorf("Documented example %q is not valid RFC3339: %v", example, err)
			}
		})
	}
}

// TestDocumentation_InvalidFormats ensures documented invalid examples actually fail.
func TestDocumentation_InvalidFormats(t *testing.T) {
	invalidExamples := []string{
		"2026-01-28",          // date-only
		"2026-01-28T10:30:00", // missing timezone
		"1704067200",          // unix timestamp
	}

	for _, example := range invalidExamples {
		t.Run(example, func(t *testing.T) {
			_, err := time.Parse(time.RFC3339, example)
			if err == nil {
				t.Errorf("Documented invalid example %q should fail RFC3339 parsing", example)
			}
		})
	}
}
