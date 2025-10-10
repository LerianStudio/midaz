// Package helpers provides reusable utilities and setup functions to streamline
// integration and end-to-end tests.
// This file contains HTTP header utilities for test requests.
package helpers

import (
	"os"
)

// AuthHeaders returns a standard set of HTTP headers for authenticated requests,
// including `Content-Type`, `X-Request-Id`, and `Authorization`.
//
// If the `TEST_AUTH_HEADER` environment variable is set, its value is used for
// the Authorization header; otherwise, a default test token is used.
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
