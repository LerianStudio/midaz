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

// TenantAuthHeaders returns headers with tenant context for multi-tenant tests.
// It generates a JWT with the specified tenant ID embedded in the claims.
// The tenant claim key is hardcoded as "tenantId".
// If tenantID is empty, standard auth headers without tenant context are returned.
func TenantAuthHeaders(requestID, tenantID string) map[string]string {
	hdr := map[string]string{
		"Content-Type": "application/json",
		"X-Request-Id": requestID,
	}

	if tenantID == "" {
		// No tenant context - use standard auth
		if v := os.Getenv("TEST_AUTH_HEADER"); v != "" {
			hdr["Authorization"] = v
		} else {
			hdr["Authorization"] = "Bearer test"
		}
		return hdr
	}

	// Generate JWT with tenant context
	token, err := GenerateTestJWT(tenantID, "", "test-user")
	if err != nil {
		// Fallback to basic auth if JWT generation fails
		if v := os.Getenv("TEST_AUTH_HEADER"); v != "" {
			hdr["Authorization"] = v
		} else {
			hdr["Authorization"] = "Bearer test"
		}
		return hdr
	}

	hdr["Authorization"] = "Bearer " + token
	return hdr
}

// TenantAuthHeadersWithSlug returns headers with tenant context including tenant slug.
// This is useful when testing with multiple tenant attributes.
func TenantAuthHeadersWithSlug(requestID, tenantID, tenantSlug, userID string) map[string]string {
	hdr := map[string]string{
		"Content-Type": "application/json",
		"X-Request-Id": requestID,
	}

	if tenantID == "" {
		if v := os.Getenv("TEST_AUTH_HEADER"); v != "" {
			hdr["Authorization"] = v
		} else {
			hdr["Authorization"] = "Bearer test"
		}
		return hdr
	}

	token, err := GenerateTestJWT(tenantID, tenantSlug, userID)
	if err != nil {
		if v := os.Getenv("TEST_AUTH_HEADER"); v != "" {
			hdr["Authorization"] = v
		} else {
			hdr["Authorization"] = "Bearer test"
		}
		return hdr
	}

	hdr["Authorization"] = "Bearer " + token
	return hdr
}
