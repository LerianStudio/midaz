// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import "testing"

// TestSanitizeServiceSegment locks the service-segment reduction used as the
// leading (ACL-scoped) topic segment: lowercase, then keep ONLY [a-z0-9] and
// drop every other rune. This replicates the tenant-manager's
// SanitizeKafkaSegment so the ACL granted on "{service}." matches byte-for-byte.
func TestSanitizeServiceSegment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain ledger", in: "ledger", want: "ledger"},
		{name: "plain tracer", in: "tracer", want: "tracer"},
		{name: "uppercase lowered", in: "Ledger", want: "ledger"},
		{name: "all caps lowered", in: "LEDGER", want: "ledger"},
		{name: "hyphen dropped", in: "fee-packages", want: "feepackages"},
		{name: "dots dropped", in: "lerian.midaz.ledger", want: "lerianmidazledger"},
		{name: "underscore dropped", in: "svc_1", want: "svc1"},
		{name: "spaces and punctuation dropped", in: "MIDAZ Ledger!", want: "midazledger"},
		{name: "empty stays empty", in: "", want: ""},
		{name: "only-symbols collapses to empty", in: "---", want: ""},
		{name: "digits kept", in: "svc123", want: "svc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sanitizeServiceSegment(tt.in)
			if got != tt.want {
				t.Fatalf("sanitizeServiceSegment(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
