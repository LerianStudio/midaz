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
