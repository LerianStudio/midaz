// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"os"
)

// AuthHeaders returns default headers including Authorization.
// If TEST_AUTH_HEADER is set, its value is used for Authorization.
func AuthHeaders() map[string]string {
	hdr := map[string]string{
		"Content-Type": "application/json",
	}
	if v := os.Getenv("TEST_AUTH_HEADER"); v != "" {
		hdr["Authorization"] = v
	} else {
		hdr["Authorization"] = "Bearer test"
	}

	return hdr
}

// AuthHeadersWithOrg returns default headers including Authorization.
// Deprecated: X-Organization-Id is no longer required. Use AuthHeaders() instead.
func AuthHeadersWithOrg(orgID string) map[string]string {
	return AuthHeaders()
}
