// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import (
	"fmt"
	"strings"
)

// defaultTenantID is the single-tenant sentinel. It mirrors the literal "default"
// used by ExtractTenantID/ResolveProvisionTenantID in field_encryptor.go: in
// single-tenant deployments the tenant resolves to this value and the mount stays
// flat (no tenant segment). In multi-tenant mode it is a reserved, non-routable
// value that MUST NOT be turned into a real sub-mount.
const defaultTenantID = "default"

// resolveMount derives the Vault Transit mount for a tenant. It is mode-aware and
// fails closed in multi-tenant mode:
//
//   - Single-tenant (multiTenant == false): always returns the flat base; the
//     tenant segment is never appended, regardless of tenantID.
//   - Multi-tenant (multiTenant == true) with a real tenant: returns base/tenantID.
//   - Multi-tenant with empty or "default" tenant: returns an error. The bare
//     multi-tenant base has no Transit engine, so resolving to it would silently
//     route to a non-existent mount. Failing closed here prevents that.
//
// Contract: base MUST already be normalized by resolveBaseMountPath (the single
// base-mount normalizer, in bootstrap). resolveMount uses base verbatim and only
// resolves the tenant segment, defensively trimming surrounding slashes/whitespace
// off the tenant.
func resolveMount(base, tenantID string, multiTenant bool) (string, error) {
	tenantID = strings.Trim(tenantID, "/ \t")

	if !multiTenant {
		return base, nil
	}

	if tenantID == "" || tenantID == defaultTenantID {
		return "", fmt.Errorf("multi-tenant mount resolution requires a real tenant id, got %q", tenantID)
	}

	return base + "/" + tenantID, nil
}
