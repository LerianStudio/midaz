// Package ptr provides pointer utility functions for the MDZ CLI.
//
// This package contains helper functions to create pointers to primitive types,
// which is useful when working with optional fields in API requests.
package ptr

// StringPtr returns a pointer to the given string.
//
// Useful for optional string fields in API requests.
//
// Parameters:
//   - s: String value
//
// Returns:
//   - *string: Pointer to the string
func StringPtr(s string) *string {
	return &s
}

// BoolPtr returns a pointer to the given boolean.
//
// Parameters:
//   - b: Boolean value
//
// Returns:
//   - *bool: Pointer to the boolean
func BoolPtr(b bool) *bool {
	return &b
}

// Float64Ptr returns a pointer to the given float64.
//
// Parameters:
//   - f64: Float64 value
//
// Returns:
//   - *float64: Pointer to the float64
func Float64Ptr(f64 float64) *float64 {
	return &f64
}

// IntPtr returns a pointer to the given int.
//
// Parameters:
//   - i: Int value
//
// Returns:
//   - *int: Pointer to the int
func IntPtr(i int) *int {
	return &i
}
