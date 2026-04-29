// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseDecimalString_FromNumber verifies that the json.Number branch of
// ParseDecimalString preserves the exact textual representation supplied by
// the caller — the precision-safe path for high-magnitude or high-precision
// decimals.
func TestParseDecimalString_FromNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    json.Number
		expected string
	}{
		{name: "high precision decimal", input: json.Number("123.456789012345"), expected: "123.456789012345"},
		{name: "large integer", input: json.Number("9999999999999999"), expected: "9999999999999999"},
		{name: "small fractional", input: json.Number("0.000001"), expected: "0.000001"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ParseDecimalString(tt.input, "0")
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestParseDecimalString_FromString verifies that string inputs are returned
// verbatim, preserving the exact representation provided by the caller.
func TestParseDecimalString_FromString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "plain integer", input: "100", expected: "100"},
		{name: "decimal value", input: "50.25", expected: "50.25"},
		{name: "zero", input: "0", expected: "0"},
		{name: "high precision", input: "123.456789012345", expected: "123.456789012345"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ParseDecimalString(tt.input, "default")
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestParseDecimalString_FromFloat64 verifies that float64 inputs are
// normalized into a decimal string via shopspring/decimal.
func TestParseDecimalString_FromFloat64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    float64
		expected string
	}{
		{name: "whole number", input: 100, expected: "100"},
		{name: "decimal", input: 50.5, expected: "50.5"},
		{name: "zero", input: 0, expected: "0"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ParseDecimalString(tt.input, "default")
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestParseDecimalString_DefaultValue verifies that unsupported or nil
// inputs fall through to the caller-provided default.
func TestParseDecimalString_DefaultValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{name: "nil input", input: nil, expected: "0"},
		{name: "int input", input: 42, expected: "0"},
		{name: "bool input", input: true, expected: "0"},
		{name: "struct input", input: struct{}{}, expected: "0"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ParseDecimalString(tt.input, "0")
			assert.Equal(t, tt.expected, got)
		})
	}
}
