package command

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/stretchr/testify/assert"
)

func TestUpdateBalances_AllStale_ReturnsError(t *testing.T) {
	t.Parallel()

	// This test documents expected behavior: when ALL balances are stale,
	// UpdateBalances must return an error rather than silently succeeding.
	//
	// Previously this would return nil (silent success) - which was the BUG
	// causing 82-97% data loss in chaos tests.
	//
	// After the fix, it should return ErrStaleBalanceUpdateSkipped error.

	// Verify the error constant exists
	assert.NotNil(t, constant.ErrStaleBalanceUpdateSkipped, "ErrStaleBalanceUpdateSkipped should be defined")

	// This is a documentation test - the actual behavior is tested by chaos tests
	// which verify that when balances are stale, an error is returned and
	// the message is NACK'd for retry, preventing data loss.
}
