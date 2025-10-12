package ptr

// StringPtr returns a pointer to the provided string value.
func StringPtr(v string) *string {
	return &v
}
