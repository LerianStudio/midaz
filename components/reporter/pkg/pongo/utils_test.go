// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNestedField(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"user": map[string]any{
			"profile": map[string]any{
				"email": "clara@example.com",
			},
		},
		"flat": "value",
	}

	t.Run("Success - existing nested field", func(t *testing.T) {
		t.Parallel()
		v, ok := getNestedField(data, "user.profile.email")
		assert.True(t, ok)
		assert.Equal(t, "clara@example.com", v)
	})

	t.Run("Success - existing flat field", func(t *testing.T) {
		t.Parallel()
		v, ok := getNestedField(data, "flat")
		assert.True(t, ok)
		assert.Equal(t, "value", v)
	})

	t.Run("Error - nonexistent nested field", func(t *testing.T) {
		t.Parallel()
		_, ok := getNestedField(data, "user.profile.phone")
		assert.False(t, ok)
	})

	t.Run("Error - invalid intermediate path", func(t *testing.T) {
		t.Parallel()
		_, ok := getNestedField(data, "user.profile.email.username")
		assert.False(t, ok)
	})

	t.Run("Error - nonexistent top-level field", func(t *testing.T) {
		t.Parallel()
		_, ok := getNestedField(data, "missing")
		assert.False(t, ok)
	})
}
