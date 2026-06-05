// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
)

// RedactConnectionString masks credentials in a connection URI.
// It replaces the username and password with "REDACTED" to prevent accidental
// credential leakage in logs. Returns "[invalid-uri]" if parsing fails.
func RedactConnectionString(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return "[invalid-uri]"
	}

	if u.User != nil {
		u.User = url.UserPassword(constant.RedactPlaceholder, constant.RedactPlaceholder)
	}

	return u.String()
}

// SanitizeExternalError strips potential credentials and connection details from
// external system error messages before they are written to logs or telemetry.
// It converts the value to a string representation and redacts anything that
// looks like a connection URI.
func SanitizeExternalError(v any) string {
	if v == nil {
		return ""
	}

	var msg string

	switch val := v.(type) {
	case error:
		msg = val.Error()
	case string:
		msg = val
	default:
		msg = fmt.Sprintf("%v", val)
	}

	// Redact anything that looks like a connection string with credentials
	if strings.Contains(msg, "://") {
		msg = redactEmbeddedURIs(msg)
	}

	return msg
}

// redactEmbeddedURIs finds and redacts URI-like patterns in a string.
func redactEmbeddedURIs(msg string) string {
	// Simple heuristic: find scheme://... patterns and redact credentials
	words := strings.Fields(msg)
	for i, word := range words {
		if strings.Contains(word, "://") {
			words[i] = RedactConnectionString(word)
		}
	}

	return strings.Join(words, " ")
}
