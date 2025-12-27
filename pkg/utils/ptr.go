package utils

// StringPtr returns a pointer to the given string value.
func StringPtr(s string) *string {
	return &s
}

// BoolPtr returns a pointer to the given bool value.
func BoolPtr(b bool) *bool {
	return &b
}

// Float64Ptr returns a pointer to the given float64 value.
func Float64Ptr(f64 float64) *float64 {
	return &f64
}

// IntPtr returns a pointer to the given int value.
func IntPtr(i int) *int {
	return &i
}
