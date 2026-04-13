// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package helpers

// OrgPayload returns a minimal valid organization payload including a valid country code.
func OrgPayload(name, legalDocument string) map[string]any {
	return map[string]any{
		"legalName":     name,
		"legalDocument": legalDocument,
		"address": map[string]any{
			"country": "US",
		},
	}
}
