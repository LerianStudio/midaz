// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package scheme normalizes the free-form scheme label a caller attaches to a
// movement (the payment rail or classification it rides on) into its single
// canonical spelling, so every component that stores or compares a scheme
// agrees on the same value. It depends on the standard library only.
package scheme

import (
	"regexp"
	"strings"
)

// MaxLength bounds a normalized scheme.
const MaxLength = 50

// pattern is the canonical shape: upper-case alphanumerics, underscore and
// dash, between one and MaxLength characters.
var pattern = regexp.MustCompile(`^[A-Z0-9_-]{1,50}$`)

// Normalize trims and upper-cases raw and reports whether the result matches
// ^[A-Z0-9_-]{1,50}$. An empty raw (after trimming) normalizes to "" and is ok,
// meaning the scheme is absent rather than invalid.
func Normalize(raw string) (value string, ok bool) {
	value = strings.ToUpper(strings.TrimSpace(raw))
	if value == "" {
		return "", true
	}

	return value, pattern.MatchString(value)
}
