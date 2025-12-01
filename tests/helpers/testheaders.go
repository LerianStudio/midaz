// Package helpers provides authentication header utilities for Midaz integration tests.
//
// # Purpose
//
// This file provides utilities for constructing authentication headers
// required for API requests in integration tests.
//
// # Environment Variables
//
//   - TEST_AUTH_HEADER: Override default Authorization header value
//
// # Default Behavior
//
// When TEST_AUTH_HEADER is not set, uses "Bearer test" as the default
// authorization token, suitable for local development environments.
//
// # Usage
//
//	headers := helpers.AuthHeaders("req-123")
//	status, body, err := client.Request(ctx, "GET", "/api/v1/users", headers, nil)
package helpers

import (
	"os"
)

// AuthHeaders returns default headers including Authorization and X-Request-Id.
//
// This function constructs the standard headers required for authenticated
// API requests in Midaz services.
//
// # Parameters
//
//   - requestID: Value for X-Request-Id header (used for request tracing)
//
// # Headers Returned
//
//   - Content-Type: "application/json"
//   - X-Request-Id: Provided requestID value
//   - Authorization: From TEST_AUTH_HEADER env var or "Bearer test"
//
// # Environment
//
// If TEST_AUTH_HEADER is set, its value is used for Authorization.
// Otherwise defaults to "Bearer test" for local development.
//
// # Returns
//
//   - map[string]string: Headers map ready for use with HTTPClient methods
//
// # Example
//
//	headers := AuthHeaders(uuid.NewString())
//	// headers = {
//	//   "Content-Type": "application/json",
//	//   "X-Request-Id": "abc-123-...",
//	//   "Authorization": "Bearer test"
//	// }
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
