// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package scheme

import (
	"strings"
	"testing"
)

func TestNormalize_TableCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantValue string
		wantOK    bool
	}{
		{name: "lowercase is upper-cased", raw: "pix", wantValue: "PIX", wantOK: true},
		{name: "mixed case with padding, digits, dash and underscore", raw: " Pix-01_x ", wantValue: "PIX-01_X", wantOK: true},
		{name: "exactly max length is accepted", raw: strings.Repeat("A", MaxLength), wantValue: strings.Repeat("A", MaxLength), wantOK: true},
		{name: "one over max length is rejected", raw: strings.Repeat("A", MaxLength+1), wantValue: strings.Repeat("A", MaxLength+1), wantOK: false},
		{name: "punctuation is rejected", raw: "pix!", wantValue: "PIX!", wantOK: false},
		{name: "inner whitespace is rejected", raw: "a b", wantValue: "A B", wantOK: false},
		{name: "empty normalizes to absent", raw: "", wantValue: "", wantOK: true},
		{name: "whitespace only normalizes to absent", raw: "   ", wantValue: "", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotValue, gotOK := Normalize(tt.raw)

			if gotValue != tt.wantValue {
				t.Errorf("Normalize(%q) value = %q, want %q", tt.raw, gotValue, tt.wantValue)
			}

			if gotOK != tt.wantOK {
				t.Errorf("Normalize(%q) ok = %v, want %v", tt.raw, gotOK, tt.wantOK)
			}
		})
	}
}

func TestMaxLength_IsFifty(t *testing.T) {
	t.Parallel()

	if MaxLength != 50 {
		t.Fatalf("MaxLength = %d, want 50", MaxLength)
	}
}
