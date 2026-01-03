package pkg

import (
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

func TestValidateBusinessError_StaleBalanceUpdateSkipped(t *testing.T) {
	t.Parallel()

	result := ValidateBusinessError(constant.ErrStaleBalanceUpdateSkipped, "Balance")

	var failedPreconditionErr FailedPreconditionError
	if !errors.As(result, &failedPreconditionErr) {
		t.Fatalf("Expected FailedPreconditionError, got %T", result)
	}

	if failedPreconditionErr.Code != "0139" {
		t.Errorf("Code = %q, want %q", failedPreconditionErr.Code, "0139")
	}

	expectedTitle := "Stale Balance Update Skipped"
	if failedPreconditionErr.Title != expectedTitle {
		t.Errorf("Title = %q, want %q", failedPreconditionErr.Title, expectedTitle)
	}
}
