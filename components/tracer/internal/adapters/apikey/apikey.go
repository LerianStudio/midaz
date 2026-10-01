// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package apikey holds the API key check shared by every tracer transport, so
// the HTTP listener and the gRPC reservation seam accept exactly the same keys.
package apikey

import (
	"crypto/subtle"
	"strings"
)

// Failure reasons Validate reports, used in logs and metrics. Neither carries
// the key.
const (
	ReasonMissing = "missing_api_key"
	ReasonInvalid = "invalid_api_key"
)

// DefaultLabel is the audit actor identifier recorded for an API-key caller
// when API_KEY_LABEL is unset.
// #nosec G101 -- audit actor identifier, not a credential value.
const DefaultLabel = "tracer-default"

// Validate compares provided against expected in constant time and returns
// the failure reason, or an empty string when the key matches. An empty
// expected key matches nothing.
func Validate(provided, expected string) string {
	if provided == "" {
		return ReasonMissing
	}

	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return ReasonInvalid
	}

	return ""
}

// ResolveLabel returns the trimmed label, or DefaultLabel when it is blank.
func ResolveLabel(label string) string {
	if trimmed := strings.TrimSpace(label); trimmed != "" {
		return trimmed
	}

	return DefaultLabel
}
