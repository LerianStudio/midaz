package ptr

func StringPtr(s string) *string {
	return &s
}

func BoolPtr(b bool) *bool {
	return &b
}

func Float64Ptr(f64 float64) *float64 {
	return &f64
}

func IntPtr(i int) *int {
	return &i
}
