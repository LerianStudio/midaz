package helpers

import (
	"os"
)

// AuthHeaders returns default headers including Authorization and X-Request-Id.
// If TEST_AUTH_HEADER is set, its value is used for Authorization.
func AuthHeaders(requestID string) map[string]string {
	hdr := map[string]string{
		"Content-Type": "application/json",
		"X-Request-Id": requestID,
	}
	if v := os.Getenv("TEST_AUTH_HEADER"); v != "" {
		hdr["Authorization"] = v
	} else {
		hdr["Authorization"] = "Bearer test"
	}

	return hdr
}

// AuthHeadersWithOrg returns headers including Authorization, X-Request-Id, and X-Organization-Id.
// This is required for CRM service endpoints that expect organization context.
func AuthHeadersWithOrg(requestID, organizationID string) map[string]string {
	hdr := AuthHeaders(requestID)
	hdr["X-Organization-Id"] = organizationID

	return hdr
}
