// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package encryption

import "strings"

// resolveMount derives the Vault Transit mount for a tenant: flat base for
// empty/"default" (single-tenant), else base/tenantID. Base and tenant are
// trimmed of surrounding slashes/whitespace.

func resolveMount(base, tenantID string) string {
	normalized := strings.Trim(base, "/")
	tenantID = strings.Trim(tenantID, "/ \t")

	if tenantID == "" || tenantID == "default" {
		return normalized
	}

	return normalized + "/" + tenantID
}
