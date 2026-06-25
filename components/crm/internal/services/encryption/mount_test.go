// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"testing"
)

func TestResolveMount(t *testing.T) {
	t.Parallel()

	// Contract: base is ALWAYS pre-normalized by resolveBaseMountPath (the single
	// base-mount normalizer in bootstrap). resolveMount uses base verbatim and only
	// resolves/defensively trims the tenant segment. Base-normalization coverage
	// lives in TestResolveBaseMountPath, not here.
	tests := []struct {
		name     string
		base     string
		tenantID string
		want     string
	}{
		{
			name:     "empty tenant returns flat base",
			base:     "transit",
			tenantID: "",
			want:     "transit",
		},
		{
			name:     "default tenant returns flat base",
			base:     "transit",
			tenantID: "default",
			want:     "transit",
		},
		{
			name:     "clean base with uuid tenant returns sub-mount",
			base:     "transit",
			tenantID: "9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
			want:     "transit/9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
		},
		{
			name:     "custom clean base with uuid tenant returns sub-mount",
			base:     "crm-transit",
			tenantID: "9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
			want:     "crm-transit/9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
		},
		{
			name:     "slash-wrapped base is NOT normalized by resolveMount (moved to resolveBaseMountPath)",
			base:     "/transit/",
			tenantID: "9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
			want:     "/transit//9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
		},
		{
			name:     "padded default sentinel resolves to flat base",
			base:     "transit",
			tenantID: " default ",
			want:     "transit",
		},
		{
			name:     "trims surrounding slashes on tenant segment",
			base:     "transit",
			tenantID: "/weird/",
			want:     "transit/weird",
		},
		{
			name:     "preserves interior slash in tenant segment",
			base:     "transit",
			tenantID: "a/b",
			want:     "transit/a/b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := resolveMount(tt.base, tt.tenantID)
			if got != tt.want {
				t.Errorf("resolveMount(%q, %q) = %q, want %q", tt.base, tt.tenantID, got, tt.want)
			}
		})
	}
}
