// Package helpers provides test utilities and helper functions for integration tests.
// This file contains HTTP header utilities for test requests.
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
