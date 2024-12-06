package mpointers

import "time"

// String just return given s as a pointer
func String(s string) *string {
	return &s
}

// Bool just return given b as a pointer
func Bool(b bool) *bool {
	return &b
}

// Time just return given t as a pointer
func Time(t time.Time) *time.Time {
	return &t
}

// Int64 just return given t as a pointer
func Int64(t int64) *int64 {
	return &t
}

// Float64 just return given t as a pointer
func Float64(t float64) *float64 {
	return &t
}

// Int just return given t as a pointer
func Int(t int) *int {
	return &t
}
