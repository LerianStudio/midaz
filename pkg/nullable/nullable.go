// Package nullable provides a generic Nullable type for handling JSON fields
// that need to distinguish between "not provided" and "explicitly set to null".
//
// This solves the common REST API problem where PATCH requests need to:
// - Ignore fields not present in the request (keep existing value)
// - Set fields to NULL when explicitly sent as null
// - Update fields when sent with a value
package nullable

import (
	"bytes"
	"encoding/json"
)

// Nullable represents a value that can be in three states:
// 1. Not set (absent from JSON) - IsSet=false, IsNull=false
// 2. Explicitly null - IsSet=true, IsNull=true
// 3. Has a value - IsSet=true, IsNull=false, Value contains the data
type Nullable[T any] struct {
	Value  T
	IsSet  bool
	IsNull bool
}

// Set creates a Nullable with a value.
func Set[T any](value T) Nullable[T] {
	return Nullable[T]{
		Value:  value,
		IsSet:  true,
		IsNull: false,
	}
}

// Null creates a Nullable that is explicitly null.
func Null[T any]() Nullable[T] {
	return Nullable[T]{
		IsSet:  true,
		IsNull: true,
	}
}

// Unset creates a Nullable that was not provided.
func Unset[T any]() Nullable[T] {
	return Nullable[T]{
		IsSet:  false,
		IsNull: false,
	}
}

// UnmarshalJSON implements json.Unmarshaler.
// This is the key method that distinguishes between null and missing values.
func (n *Nullable[T]) UnmarshalJSON(data []byte) error {
	// If we're being called, the field was present in the JSON
	n.IsSet = true

	// Check if the value is explicitly null
	if bytes.Equal(data, []byte("null")) {
		n.IsNull = true

		return nil
	}

	// Otherwise, unmarshal the value
	n.IsNull = false

	return json.Unmarshal(data, &n.Value)
}

// MarshalJSON implements json.Marshaler.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if !n.IsSet || n.IsNull {
		return []byte("null"), nil
	}

	return json.Marshal(n.Value)
}

// Get returns the value and a boolean indicating if it's valid (set and not null).
func (n Nullable[T]) Get() (T, bool) {
	if n.IsSet && !n.IsNull {
		return n.Value, true
	}

	var zero T

	return zero, false
}

// GetOrDefault returns the value if set and not null, otherwise returns the default.
func (n Nullable[T]) GetOrDefault(defaultValue T) T {
	if n.IsSet && !n.IsNull {
		return n.Value
	}

	return defaultValue
}

// ToPointer converts to a pointer (nil if not set or null, pointer to value otherwise).
// Useful for compatibility with existing code that uses *string, *bool, etc.
func (n Nullable[T]) ToPointer() *T {
	if n.IsSet && !n.IsNull {
		return &n.Value
	}

	return nil
}

// ShouldUpdate returns true if this field should be included in an UPDATE query.
// This is the key method for solving the PATCH problem.
func (n Nullable[T]) ShouldUpdate() bool {
	return n.IsSet
}

// ShouldSetNull returns true if this field should be SET to NULL.
func (n Nullable[T]) ShouldSetNull() bool {
	return n.IsSet && n.IsNull
}
