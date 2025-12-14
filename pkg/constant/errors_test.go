package constant

import (
	"testing"
)

func TestErrStaleBalanceUpdateSkipped_Exists(t *testing.T) {
	t.Parallel()

	if ErrStaleBalanceUpdateSkipped == nil {
		t.Fatal("ErrStaleBalanceUpdateSkipped should be defined")
	}

	expected := "0129"
	if ErrStaleBalanceUpdateSkipped.Error() != expected {
		t.Errorf("ErrStaleBalanceUpdateSkipped.Error() = %q, want %q", ErrStaleBalanceUpdateSkipped.Error(), expected)
	}
}
