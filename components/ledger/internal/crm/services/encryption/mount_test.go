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
	//
	// resolveMount is mode-aware:
	//   - single-tenant (multiTenant == false): flat base, tenant never appended.
	//   - multi-tenant (multiTenant == true): base/tenant for a real tenant; empty
	//     or "default" fails closed (no Transit engine on the bare base).
	tests := []struct {
		name        string
		base        string
		tenantID    string
		multiTenant bool
		want        string
		wantErr     bool
	}{
		// Single-tenant: flat base regardless of tenant value.
		{
			name:        "single-tenant empty tenant returns flat base",
			base:        "transit",
			tenantID:    "",
			multiTenant: false,
			want:        "transit",
		},
		{
			name:        "single-tenant default tenant returns flat base",
			base:        "transit",
			tenantID:    "default",
			multiTenant: false,
			want:        "transit",
		},
		{
			name:        "single-tenant real tenant still returns flat base",
			base:        "transit",
			tenantID:    "9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
			multiTenant: false,
			want:        "transit",
		},
		// Multi-tenant: real tenant resolves to a sub-mount.
		{
			name:        "multi-tenant clean base with uuid tenant returns sub-mount",
			base:        "transit",
			tenantID:    "9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
			multiTenant: true,
			want:        "transit/9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
		},
		{
			name:        "multi-tenant custom clean base with uuid tenant returns sub-mount",
			base:        "crm-transit",
			tenantID:    "9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
			multiTenant: true,
			want:        "crm-transit/9b2e4c1a-7f60-4d3e-8a21-1c5b0e7d9f44",
		},
		{
			name:        "multi-tenant trims surrounding slashes on tenant segment",
			base:        "transit",
			tenantID:    "/weird/",
			multiTenant: true,
			want:        "transit/weird",
		},
		{
			name:        "multi-tenant preserves interior slash in tenant segment",
			base:        "transit",
			tenantID:    "a/b",
			multiTenant: true,
			want:        "transit/a/b",
		},
		// Multi-tenant fail-closed: empty/"default" must NOT resolve to the bare base.
		{
			name:        "multi-tenant empty tenant fails closed",
			base:        "transit",
			tenantID:    "",
			multiTenant: true,
			wantErr:     true,
		},
		{
			name:        "multi-tenant default tenant fails closed",
			base:        "transit",
			tenantID:    "default",
			multiTenant: true,
			wantErr:     true,
		},
		{
			name:        "multi-tenant padded default sentinel fails closed",
			base:        "transit",
			tenantID:    " default ",
			multiTenant: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveMount(tt.base, tt.tenantID, tt.multiTenant)
			if tt.wantErr {
				if err == nil {
					t.Errorf("resolveMount(%q, %q, %t) expected error, got nil (result %q)", tt.base, tt.tenantID, tt.multiTenant, got)
				}

				if got != "" {
					t.Errorf("resolveMount(%q, %q, %t) on error = %q, want empty string", tt.base, tt.tenantID, tt.multiTenant, got)
				}

				return
			}

			if err != nil {
				t.Errorf("resolveMount(%q, %q, %t) unexpected error: %v", tt.base, tt.tenantID, tt.multiTenant, err)
			}

			if got != tt.want {
				t.Errorf("resolveMount(%q, %q, %t) = %q, want %q", tt.base, tt.tenantID, tt.multiTenant, got, tt.want)
			}
		})
	}
}
