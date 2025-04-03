package ptr

// \1 performs an operation
func StringPtr(s string) *string {
	return &s
}

// \1 performs an operation
func BoolPtr(b bool) *bool {
	return &b
}

// \1 performs an operation
func Float64Ptr(f64 float64) *float64 {
	return &f64
}

// \1 performs an operation
func IntPtr(i int) *int {
	return &i
}
