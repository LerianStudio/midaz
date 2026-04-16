// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAccountHandler_SanitizesAliasInLogs verifies the local sanitizeLogToken
// helper strips log-injection vectors before the alias is embedded in a log
// line. Prior to D10-B6 an attacker that controlled the :alias path param
// could forge log entries by sending values containing CR/LF — Midaz log
// shippers ingest line-delimited JSON and a forged "\n{"level":"error"}"
// payload would appear as an independent log event in Datadog/ES.
func TestAccountHandler_SanitizesAliasInLogs(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantEqual string
		wantNo    []string
	}{
		{
			name:      "newline injection",
			input:     "line1\nFORGED LOG",
			wantEqual: "line1FORGED LOG",
			wantNo:    []string{"\n", "\r"},
		},
		{
			name:      "carriage return injection",
			input:     "alias\rFORGED",
			wantEqual: "aliasFORGED",
			wantNo:    []string{"\r", "\n"},
		},
		{
			name:      "tab injection",
			input:     "a\tb",
			wantEqual: "ab",
			wantNo:    []string{"\t"},
		},
		{
			name:      "CRLF injection",
			input:     "alias\r\nmalicious",
			wantEqual: "aliasmalicious",
			wantNo:    []string{"\r\n", "\r", "\n"},
		},
		{
			name:      "C1 control characters",
			input:     "a\u0080b\u009Fc",
			wantEqual: "abc",
		},
		{
			name:      "zero-width characters",
			input:     "a\u200Bb\u200Cc\u200Dd\uFEFFe",
			wantEqual: "abcde",
		},
		{
			name:      "DEL character",
			input:     "a\u007Fb",
			wantEqual: "ab",
		},
		{
			name:      "empty input",
			input:     "",
			wantEqual: "unknown",
		},
		{
			name:      "whitespace only",
			input:     "   \t\n ",
			wantEqual: "unknown",
		},
		{
			name:      "all zero-width",
			input:     "\u200B\u200C\u200D\uFEFF",
			wantEqual: "unknown",
		},
		{
			name:      "trim leading/trailing spaces but keep internal",
			input:     "  my-alias  ",
			wantEqual: "my-alias",
		},
		{
			name:      "legitimate alias with symbols passes through",
			input:     "@person.one_v2",
			wantEqual: "@person.one_v2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeLogToken(tc.input)
			require.Equal(t, tc.wantEqual, got)

			for _, bad := range tc.wantNo {
				require.NotContainsf(t, got, bad,
					"sanitized token must not retain %q (input=%q, got=%q)", bad, tc.input, got)
			}
		})
	}
}

// TestSanitizeLogToken_CapsLength ensures very long user input is truncated to
// prevent log-line-amplification attacks (e.g. a 10MB alias value filling the
// log pipeline).
func TestSanitizeLogToken_CapsLength(t *testing.T) {
	long := strings.Repeat("a", 500)
	got := sanitizeLogToken(long)
	require.Len(t, got, maxAccountLogTokenLength)
	require.Equal(t, long[:maxAccountLogTokenLength], got)
}
