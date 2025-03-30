package utils

import (
	"strconv"
)

// StringToInt64 converts a string to an int64.
// Returns the converted value and any error that occurred.
func StringToInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
