// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactConnectionString_StandardURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "AMQP URI with user and password",
			uri:      "amqp://user:pass@host:5672",
			expected: "amqp://REDACTED:REDACTED@host:5672",
		},
		{
			name:     "MongoDB URI with user and password",
			uri:      "mongodb://admin:secret@host:27017",
			expected: "mongodb://REDACTED:REDACTED@host:27017",
		},
		{
			name:     "URI with path and parameters",
			uri:      "mongodb://user:pass@host:27017/?authSource=admin",
			expected: "mongodb://REDACTED:REDACTED@host:27017/?authSource=admin",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := RedactConnectionString(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRedactConnectionString_NoCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "MongoDB URI without credentials",
			uri:      "mongodb://host:27017",
			expected: "mongodb://host:27017",
		},
		{
			name:     "Simple host URI",
			uri:      "redis://localhost:6379",
			expected: "redis://localhost:6379",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := RedactConnectionString(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRedactConnectionString_SpecialCharactersInPassword(t *testing.T) {
	t.Parallel()

	result := RedactConnectionString("mongodb://user:p%40ss@host:27017")
	assert.Equal(t, "mongodb://REDACTED:REDACTED@host:27017", result)
}

func TestRedactConnectionString_EmptyString(t *testing.T) {
	t.Parallel()

	result := RedactConnectionString("")
	assert.Equal(t, "", result)
}

func TestRedactConnectionString_InvalidURI(t *testing.T) {
	t.Parallel()

	result := RedactConnectionString("://missing-scheme")
	assert.Equal(t, "[invalid-uri]", result)
}

func TestRedactConnectionString_UsernameOnly(t *testing.T) {
	t.Parallel()

	result := RedactConnectionString("amqp://onlyuser@host:5672")
	assert.Equal(t, "amqp://REDACTED:REDACTED@host:5672", result)
}
