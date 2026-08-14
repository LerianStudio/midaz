// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package sanitize

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetadataSanitizer(t *testing.T) {
	tests := []struct {
		name         string
		patterns     []string
		wantPatterns int
	}{
		{
			name:         "creates sanitizer with valid patterns",
			patterns:     []string{`(?i)password`, `(?i)token`},
			wantPatterns: 2,
		},
		{
			name:         "skips invalid regex patterns",
			patterns:     []string{`(?i)password`, `[invalid`, `(?i)token`},
			wantPatterns: 2, // invalid pattern skipped
		},
		{
			name:         "handles empty patterns",
			patterns:     []string{},
			wantPatterns: 0,
		},
		{
			name:         "handles nil patterns",
			patterns:     nil,
			wantPatterns: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewMetadataSanitizer(tt.patterns)

			require.NotNil(t, s)
			assert.Len(t, s.patterns, tt.wantPatterns)
		})
	}
}

func TestMetadataSanitizer_Sanitize(t *testing.T) {
	sanitizer := NewMetadataSanitizer(DefaultSensitivePatterns)

	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:     "nil input returns nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map returns empty map",
			input:    map[string]any{},
			expected: map[string]any{},
		},
		{
			name: "non-sensitive keys pass through",
			input: map[string]any{
				"transaction_id": "txn-123",
				"amount":         1000,
				"currency":       "USD",
			},
			expected: map[string]any{
				"transaction_id": "txn-123",
				"amount":         1000,
				"currency":       "USD",
			},
		},
		{
			name: "password key is redacted",
			input: map[string]any{
				"username": "john",
				"password": "secret123",
			},
			expected: map[string]any{
				"username": "john",
				"password": MaskedValue,
			},
		},
		{
			name: "case insensitive matching",
			input: map[string]any{
				"PASSWORD":    "secret1",
				"Password":    "secret2",
				"passWord":    "secret3",
				"user_passwd": "secret4",
			},
			expected: map[string]any{
				"PASSWORD":    MaskedValue,
				"Password":    MaskedValue,
				"passWord":    MaskedValue,
				"user_passwd": MaskedValue,
			},
		},
		{
			name: "authentication keys redacted",
			input: map[string]any{
				"api_key":     "ak-123",
				"auth_token":  "tok-456",
				"bearer":      "bearer-789",
				"secret_key":  "sk-abc",
				"credentials": "creds",
			},
			expected: map[string]any{
				"api_key":     MaskedValue,
				"auth_token":  MaskedValue,
				"bearer":      MaskedValue,
				"secret_key":  MaskedValue,
				"credentials": MaskedValue,
			},
		},
		{
			name: "PII keys redacted",
			input: map[string]any{
				"email":           "john@example.com",
				"phone_number":    "555-1234",
				"ssn":             "123-45-6789",
				"cpf":             "123.456.789-00",
				"address":         "123 Main St",
				"full_name":       "John Doe",
				"date_of_birth":   "1990-01-01",
				"passport_number": "AB123456",
			},
			expected: map[string]any{
				"email":           MaskedValue,
				"phone_number":    MaskedValue,
				"ssn":             MaskedValue,
				"cpf":             MaskedValue,
				"address":         MaskedValue,
				"full_name":       MaskedValue,
				"date_of_birth":   MaskedValue,
				"passport_number": MaskedValue,
			},
		},
		{
			name: "financial keys redacted",
			input: map[string]any{
				"card_number":    "4111111111111111",
				"cvv":            "123",
				"account_number": "123456789",
				"routing_number": "021000021",
				"iban":           "GB82WEST12345698765432",
				"pin":            "1234",
			},
			expected: map[string]any{
				"card_number":    MaskedValue,
				"cvv":            MaskedValue,
				"account_number": MaskedValue,
				"routing_number": MaskedValue,
				"iban":           MaskedValue,
				"pin":            MaskedValue,
			},
		},
		{
			name: "nested maps are recursively sanitized",
			input: map[string]any{
				"user": map[string]any{
					"id":       "user-123",
					"email":    "john@example.com",
					"password": "secret",
				},
				"metadata": map[string]any{
					"api_key": "ak-123",
					"region":  "us-east-1",
				},
			},
			expected: map[string]any{
				"user": map[string]any{
					"id":       "user-123",
					"email":    MaskedValue,
					"password": MaskedValue,
				},
				"metadata": map[string]any{
					"api_key": MaskedValue,
					"region":  "us-east-1",
				},
			},
		},
		{
			name: "deeply nested maps",
			input: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"password": "deep-secret",
							"value":    "safe",
						},
					},
				},
			},
			expected: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"password": MaskedValue,
							"value":    "safe",
						},
					},
				},
			},
		},
		{
			name: "slices containing maps are sanitized",
			input: map[string]any{
				"users": []any{
					map[string]any{"name": "John", "email": "john@test.com"},
					map[string]any{"name": "Jane", "password": "secret"},
				},
			},
			expected: map[string]any{
				"users": []any{
					map[string]any{"name": "John", "email": MaskedValue},
					map[string]any{"name": "Jane", "password": MaskedValue},
				},
			},
		},
		{
			name: "slices of primitives pass through",
			input: map[string]any{
				"tags":   []any{"tag1", "tag2", "tag3"},
				"counts": []any{1, 2, 3},
			},
			expected: map[string]any{
				"tags":   []any{"tag1", "tag2", "tag3"},
				"counts": []any{1, 2, 3},
			},
		},
		{
			name: "mixed content",
			input: map[string]any{
				"safe_field":   "safe_value",
				"password":     "secret",
				"nested_safe":  map[string]any{"key": "value"},
				"nested_risky": map[string]any{"api_key": "key123"},
				"array_safe":   []any{"a", "b"},
				"array_risky":  []any{map[string]any{"token": "tok123"}},
			},
			expected: map[string]any{
				"safe_field":   "safe_value",
				"password":     MaskedValue,
				"nested_safe":  map[string]any{"key": "value"},
				"nested_risky": map[string]any{"api_key": MaskedValue},
				"array_safe":   []any{"a", "b"},
				"array_risky":  []any{map[string]any{"token": MaskedValue}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizer.Sanitize(tt.input)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMetadataSanitizer_Sanitize_DoesNotModifyOriginal(t *testing.T) {
	sanitizer := NewMetadataSanitizer(DefaultSensitivePatterns)

	original := map[string]any{
		"password": "original-secret",
		"nested": map[string]any{
			"api_key": "original-key",
		},
	}

	// Store original values
	originalPassword := original["password"]
	originalNested := original["nested"].(map[string]any)
	originalAPIKey := originalNested["api_key"]

	// Sanitize
	result := sanitizer.Sanitize(original)

	// Verify result is sanitized
	assert.Equal(t, MaskedValue, result["password"])
	assert.Equal(t, MaskedValue, result["nested"].(map[string]any)["api_key"])

	// Verify original is NOT modified
	assert.Equal(t, originalPassword, original["password"])
	assert.Equal(t, originalAPIKey, original["nested"].(map[string]any)["api_key"])
}

func TestMetadataSanitizer_IsSensitiveKey(t *testing.T) {
	sanitizer := NewMetadataSanitizer(DefaultSensitivePatterns)

	tests := []struct {
		key      string
		expected bool
	}{
		// Sensitive keys
		{"password", true},
		{"PASSWORD", true},
		{"user_password", true},
		{"api_key", true},
		{"apiKey", true},
		{"auth_token", true},
		{"secret", true},
		{"ssn", true},
		{"email", true},
		{"phone", true},
		{"card_number", true},
		{"cvv", true},

		// Safe keys
		{"transaction_id", false},
		{"amount", false},
		{"currency", false},
		{"timestamp", false},
		{"status", false},
		{"decision", false},
		{"rule_id", false},
		{"merchant_category", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := sanitizer.IsSensitiveKey(tt.key)

			assert.Equal(t, tt.expected, result, "key: %s", tt.key)
		})
	}
}

func TestMetadataSanitizer_SanitizeValue(t *testing.T) {
	sanitizer := NewMetadataSanitizer(DefaultSensitivePatterns)

	tests := []struct {
		key      string
		value    any
		expected any
	}{
		{"password", "secret123", MaskedValue},
		{"email", "test@example.com", MaskedValue},
		{"amount", 1000, 1000},
		{"status", "active", "active"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := sanitizer.SanitizeValue(tt.key, tt.value)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefault(t *testing.T) {
	s1 := Default()
	s2 := Default()

	// Should return the same instance
	assert.Same(t, s1, s2)
	assert.NotNil(t, s1)
}

func TestSanitizeMetadata(t *testing.T) {
	input := map[string]any{
		"password": "secret",
		"amount":   1000,
	}

	result := SanitizeMetadata(input)

	assert.Equal(t, MaskedValue, result["password"])
	assert.Equal(t, 1000, result["amount"])
}

func TestIsNonSerializable(t *testing.T) {
	t.Parallel()

	t.Run("returns true for channels", func(t *testing.T) {
		ch := make(chan int)
		assert.True(t, IsNonSerializable(ch), "channel should be non-serializable")
	})

	t.Run("returns true for functions", func(t *testing.T) {
		fn := func() {}
		assert.True(t, IsNonSerializable(fn), "function should be non-serializable")
	})

	t.Run("returns true for complex numbers", func(t *testing.T) {
		c64 := complex(float32(1), float32(2))
		c128 := complex(float64(1), float64(2))
		assert.True(t, IsNonSerializable(c64), "complex64 should be non-serializable")
		assert.True(t, IsNonSerializable(c128), "complex128 should be non-serializable")
	})

	t.Run("returns true for cyclic references", func(t *testing.T) {
		// Create a cyclic structure using a pointer
		type Node struct {
			Value int
			Next  *Node
		}
		node := &Node{Value: 1}
		node.Next = node // cyclic reference

		assert.True(t, IsNonSerializable(node), "cyclic reference should be non-serializable")
	})

	t.Run("returns false for serializable types", func(t *testing.T) {
		assert.False(t, IsNonSerializable("string"), "string should be serializable")
		assert.False(t, IsNonSerializable(123), "int should be serializable")
		assert.False(t, IsNonSerializable(12.34), "float should be serializable")
		assert.False(t, IsNonSerializable(true), "bool should be serializable")
		assert.False(t, IsNonSerializable([]int{1, 2, 3}), "slice should be serializable")
		assert.False(t, IsNonSerializable(map[string]int{"a": 1}), "map should be serializable")
		assert.False(t, IsNonSerializable(nil), "nil should be serializable")
	})

	t.Run("returns false for struct without non-serializable fields", func(t *testing.T) {
		type Simple struct {
			Name  string
			Value int
		}
		s := Simple{Name: "test", Value: 42}
		assert.False(t, IsNonSerializable(s), "simple struct should be serializable")
	})

	t.Run("returns true for struct with non-serializable field", func(t *testing.T) {
		type WithChannel struct {
			Name string
			Ch   chan int
		}
		s := WithChannel{Name: "test", Ch: make(chan int)}
		assert.True(t, IsNonSerializable(s), "struct with channel field should be non-serializable")
	})

	t.Run("returns false for nil pointer", func(t *testing.T) {
		var ptr *int
		assert.False(t, IsNonSerializable(ptr), "nil pointer should be serializable")
	})

	t.Run("returns false for valid pointer", func(t *testing.T) {
		val := 42
		assert.False(t, IsNonSerializable(&val), "pointer to int should be serializable")
	})

	t.Run("handles interface containing non-serializable", func(t *testing.T) {
		var i any = make(chan int)
		assert.True(t, IsNonSerializable(i), "interface containing channel should be non-serializable")
	})

	t.Run("handles nil interface", func(t *testing.T) {
		var i any
		assert.False(t, IsNonSerializable(i), "nil interface should be serializable")
	})
}

func TestMergePatterns(t *testing.T) {
	tests := []struct {
		name     string
		lists    [][]string
		expected []string
	}{
		{
			name:     "merges two lists",
			lists:    [][]string{{"a", "b"}, {"c", "d"}},
			expected: []string{"a", "b", "c", "d"},
		},
		{
			name:     "removes duplicates",
			lists:    [][]string{{"a", "b"}, {"b", "c"}},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "handles empty lists",
			lists:    [][]string{{}, {"a"}},
			expected: []string{"a"},
		},
		{
			name:     "trims whitespace",
			lists:    [][]string{{"  a  ", " b"}, {"c "}},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "skips empty strings",
			lists:    [][]string{{"a", "", "b"}, {"", "c"}},
			expected: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergePatterns(tt.lists...)

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitize_NilSlice(t *testing.T) {
	sanitizer := NewMetadataSanitizer(DefaultSensitivePatterns)

	input := map[string]any{
		"items": ([]any)(nil),
	}

	result := sanitizer.Sanitize(input)

	assert.Nil(t, result["items"])
}

func TestSanitize_NestedSlicesWithMaps(t *testing.T) {
	sanitizer := NewMetadataSanitizer(DefaultSensitivePatterns)

	input := map[string]any{
		"outer": []any{
			[]any{
				map[string]any{"password": "deep-secret"},
			},
		},
	}

	result := sanitizer.Sanitize(input)

	outer := result["outer"].([]any)
	inner := outer[0].([]any)
	innerMap := inner[0].(map[string]any)

	assert.Equal(t, MaskedValue, innerMap["password"])
}

// Benchmark for performance verification
func BenchmarkSanitize(b *testing.B) {
	sanitizer := NewMetadataSanitizer(DefaultSensitivePatterns)

	input := map[string]any{
		"transaction_id": "txn-123456789",
		"amount":         100,
		"currency":       "USD",
		"password":       "secret123",
		"api_key":        "ak-abcdef",
		"user": map[string]any{
			"id":    "user-123",
			"email": "test@example.com",
			"name":  "John Doe",
		},
		"metadata": map[string]any{
			"source":     "api",
			"auth_token": "tok-123",
		},
	}

	b.ResetTimer()

	for b.Loop() {
		_ = sanitizer.Sanitize(input)
	}
}

// BenchmarkIsSensitiveKey measures key matching performance
func BenchmarkIsSensitiveKey(b *testing.B) {
	sanitizer := NewMetadataSanitizer(DefaultSensitivePatterns)

	keys := []string{
		"transaction_id",
		"password",
		"amount",
		"api_key",
		"currency",
		"email",
	}

	b.ResetTimer()

	for b.Loop() {
		for _, key := range keys {
			_ = sanitizer.IsSensitiveKey(key)
		}
	}
}
