package testutils

// Ptr returns a pointer to the given value.
// This is useful for creating pointers to literals in test cases.
//
// Example:
//
//	alias := &mmodel.Alias{
//	    Document: testutils.Ptr("12345678901"),
//	    Type:     testutils.Ptr("NATURAL_PERSON"),
//	}
func Ptr[T any](v T) *T {
	return &v
}
