// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import "strings"

// resolveMount derives the Vault Transit mount for a tenant: flat base for
// empty/"default" (single-tenant), else base/tenantID.
//
// Contract: base MUST already be normalized by resolveBaseMountPath (the single
// base-mount normalizer, in bootstrap). resolveMount uses base verbatim and only
// resolves the tenant segment, defensively trimming surrounding slashes/whitespace
// off the tenant. The strings import remains in use for that tenant trim.
func resolveMount(base, tenantID string) string {
	tenantID = strings.Trim(tenantID, "/ \t")

	if tenantID == "" || tenantID == "default" {
		return base
	}

	return base + "/" + tenantID
}
