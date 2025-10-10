// Package helpers provides reusable utilities and setup functions to streamline
// integration and end-to-end tests.
// This file contains test payload generation utilities for API testing.
package helpers

// OrgPayload returns a minimal, valid payload for creating an organization,
// including a default country code.
func OrgPayload(name, legalDocument string) map[string]any {
	return map[string]any{
		"legalName":     name,
		"legalDocument": legalDocument,
		"address": map[string]any{
			"country": "US",
		},
	}
}
