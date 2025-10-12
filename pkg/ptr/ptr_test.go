package ptr

import "testing"

func TestStringPtr(t *testing.T) {
	s := "hello"
	p := StringPtr(s)
	if p == nil {
		t.Fatalf("StringPtr returned nil")
	}
	if *p != s {
		t.Fatalf("StringPtr value mismatch: want %q got %q", s, *p)
	}

	// Ensure pointer points to independent storage (modifying original should not change pointer value)
	s = "world"
	if *p != "hello" {
		t.Fatalf("StringPtr should keep original value: got %q", *p)
	}
}
