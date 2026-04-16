// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package dbpool

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// errSentinelTest is a stable test-only sentinel to satisfy the err113
// lint rule in this file's error-wrapping tests.
var errSentinelTest = errors.New("sentinel test error")

func TestScrubDSN_RemovesPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		dsn         string
		mustAbsent  string
		mustPresent []string
	}{
		{
			name:        "uri form with password",
			dsn:         "postgres://midaz:supersecret@localhost:5432/ledger?sslmode=disable",
			mustAbsent:  "supersecret",
			mustPresent: []string{"midaz", "localhost", "5432", "ledger", "***"},
		},
		{
			name:        "postgresql scheme",
			dsn:         "postgresql://u:p@h/d",
			mustAbsent:  ":p@",
			mustPresent: []string{"u", "h", "d", "***"},
		},
		{
			name:        "keyword form with password",
			dsn:         "host=localhost user=midaz password=supersecret dbname=ledger port=5432 sslmode=disable",
			mustAbsent:  "supersecret",
			mustPresent: []string{"host=localhost", "user=midaz", "password=***", "dbname=ledger"},
		},
		{
			name:        "keyword form with quoted password",
			dsn:         "host=h user=u password='weird value' dbname=d",
			mustAbsent:  "weird value",
			mustPresent: []string{"password=***", "dbname=d"},
		},
		{
			name:        "uri without password preserved",
			dsn:         "postgres://midaz@localhost/ledger",
			mustAbsent:  "***",
			mustPresent: []string{"midaz", "ledger"},
		},
		{
			name:        "empty returns empty",
			dsn:         "",
			mustAbsent:  "***",
			mustPresent: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ScrubDSN(tc.dsn)

			if tc.mustAbsent != "" && strings.Contains(got, tc.mustAbsent) {
				t.Fatalf("scrubbed DSN still contains %q: %q", tc.mustAbsent, got)
			}

			for _, needle := range tc.mustPresent {
				if !strings.Contains(got, needle) {
					t.Fatalf("scrubbed DSN missing %q: %q", needle, got)
				}
			}
		})
	}
}

func TestScrubDSNInError(t *testing.T) {
	t.Parallel()

	dsn := "host=h user=u password=supersecret dbname=d"

	t.Run("substitutes dsn in error", func(t *testing.T) {
		t.Parallel()

		base := fmt.Errorf("connect failed: %s: %w", dsn, errSentinelTest)
		got := ScrubDSNInError(base, dsn)

		if strings.Contains(got.Error(), "supersecret") {
			t.Fatalf("error still contains password: %q", got.Error())
		}

		if !strings.Contains(got.Error(), "password=***") {
			t.Fatalf("error missing redacted marker: %q", got.Error())
		}
	})

	t.Run("no-op when dsn absent", func(t *testing.T) {
		t.Parallel()

		base := fmt.Errorf("other failure: %w", errSentinelTest)

		got := ScrubDSNInError(base, dsn)
		if !errors.Is(got, errSentinelTest) {
			t.Fatalf("expected wrapped sentinel preserved, got %v", got)
		}

		if got.Error() != base.Error() {
			t.Fatalf("expected identical message when dsn not present, got %q want %q", got.Error(), base.Error())
		}
	})

	t.Run("nil error returns nil", func(t *testing.T) {
		t.Parallel()

		if got := ScrubDSNInError(nil, dsn); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}
