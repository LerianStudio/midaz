// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import "strings"

// resolveMount derives the Vault Transit mount path for a tenant.
// The base is normalized by trimming surrounding slashes. For empty or
// "default" tenants it returns the flat base (single-tenant unchanged);
// otherwise it returns a per-tenant sub-mount of the form base/tenantID.
func resolveMount(base, tenantID string) string {
	normalized := strings.Trim(base, "/")

	if tenantID == "" || tenantID == "default" {
		return normalized
	}

	return normalized + "/" + tenantID
}
