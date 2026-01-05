package utils

import "time"

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

// SafeTimePtr safely dereferences a time pointer, returning zero time if nil
func SafeTimePtr(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}

	return *t
}
