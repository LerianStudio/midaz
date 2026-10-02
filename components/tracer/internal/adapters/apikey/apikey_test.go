// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package apikey

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	const expected = "a-32-character-api-key-for-tests"

	tests := []struct {
		name     string
		provided string
		want     string
	}{
		{name: "missing key", provided: "", want: ReasonMissing},
		{name: "wrong key", provided: "not-the-key", want: ReasonInvalid},
		{name: "prefix of the key", provided: expected[:10], want: ReasonInvalid},
		{name: "right key", provided: expected, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Validate(tt.provided, expected))
		})
	}
}

func TestValidate_EmptyExpectedKeyNeverMatches(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ReasonInvalid, Validate("anything", ""))
	assert.Equal(t, ReasonMissing, Validate("", ""))
}

func TestResolveLabel(t *testing.T) {
	t.Parallel()

	assert.Equal(t, DefaultLabel, ResolveLabel(""))
	assert.Equal(t, DefaultLabel, ResolveLabel("   "))
	assert.Equal(t, "ledger-prod", ResolveLabel(" ledger-prod "))
	assert.Equal(t, "tracer-default", DefaultLabel)
}
