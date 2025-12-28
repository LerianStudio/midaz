package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringPtr(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty string", input: ""},
		{name: "simple string", input: "hello"},
		{name: "string with spaces", input: "hello world"},
		{name: "unicode string", input: "hello 世界"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringPtr(tt.input)
			assert.NotNil(t, result)
			assert.Equal(t, tt.input, *result)
		})
	}
}

func TestBoolPtr(t *testing.T) {
	tests := []struct {
		name  string
		input bool
	}{
		{name: "true", input: true},
		{name: "false", input: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BoolPtr(tt.input)
			assert.NotNil(t, result)
			assert.Equal(t, tt.input, *result)
		})
	}
}

func TestFloat64Ptr(t *testing.T) {
	tests := []struct {
		name  string
		input float64
	}{
		{name: "zero", input: 0.0},
		{name: "positive", input: 123.456},
		{name: "negative", input: -789.012},
		{name: "very small", input: 0.000001},
		{name: "very large", input: 1e10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Float64Ptr(tt.input)
			assert.NotNil(t, result)
			assert.Equal(t, tt.input, *result)
		})
	}
}

func TestIntPtr(t *testing.T) {
	tests := []struct {
		name  string
		input int
	}{
		{name: "zero", input: 0},
		{name: "positive", input: 42},
		{name: "negative", input: -100},
		{name: "max int32", input: 2147483647},
		{name: "min int32", input: -2147483648},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IntPtr(tt.input)
			assert.NotNil(t, result)
			assert.Equal(t, tt.input, *result)
		})
	}
}
