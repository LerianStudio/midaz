// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import (
	"strings"
	"testing"
)

// FuzzActionValidation verifies that action validation logic handles arbitrary strings safely.
func FuzzActionValidation(f *testing.F) {
	// Seed corpus: valid actions
	f.Add("direct")
	f.Add("hold")
	f.Add("commit")
	f.Add("cancel")
	f.Add("revert")

	// Edge cases
	f.Add("")
	f.Add("DIRECT")
	f.Add("Direct")
	f.Add("unknown")
	f.Add("direct ")
	f.Add(" direct")
	f.Add("direct\n")
	f.Add("direct\x00")

	validSet := make(map[string]bool, len(ValidActions))
	for _, a := range ValidActions {
		validSet[a] = true
	}

	f.Fuzz(func(t *testing.T, action string) {
		isValid := validSet[action]

		// Property: valid actions are always lowercase
		if isValid && action != strings.ToLower(action) {
			t.Errorf("valid action %q is not lowercase", action)
		}

		// Property: valid actions are never empty
		if isValid && action == "" {
			t.Error("empty string should not be a valid action")
		}

		// Property: valid actions contain no whitespace
		if isValid && strings.TrimSpace(action) != action {
			t.Errorf("valid action %q contains leading/trailing whitespace", action)
		}

		// Property: ValidActions slice is always consistent with validSet
		if len(validSet) != len(ValidActions) {
			t.Errorf("validSet size %d != ValidActions size %d", len(validSet), len(ValidActions))
		}
	})
}
