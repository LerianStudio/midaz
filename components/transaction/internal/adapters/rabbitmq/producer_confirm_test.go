package rabbitmq

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestPublisherConfirmConstants validates that publisher confirm configuration
// constants are defined with reasonable values for production use.
func TestPublisherConfirmConstants(t *testing.T) {
	t.Parallel()

	t.Run("publishConfirmTimeout is within acceptable range", func(t *testing.T) {
		t.Parallel()

		// The timeout should be long enough to handle temporary broker unavailability
		// during chaos scenarios, but not so long that it causes excessive delays
		// in normal operations.
		//
		// Range: 5s minimum (allow broker to recover from brief hiccups)
		//        30s maximum (prevent excessive blocking in failure scenarios)
		minTimeout := 5 * time.Second
		maxTimeout := 30 * time.Second

		assert.GreaterOrEqual(t, publishConfirmTimeout, minTimeout,
			"publishConfirmTimeout should be at least %v to handle temporary broker unavailability", minTimeout)
		assert.LessOrEqual(t, publishConfirmTimeout, maxTimeout,
			"publishConfirmTimeout should be at most %v to prevent excessive blocking", maxTimeout)
	})

	t.Run("publishConfirmTimeout has expected value", func(t *testing.T) {
		t.Parallel()

		// 10 seconds is the expected value - long enough to handle temporary
		// broker unavailability during chaos scenarios while not blocking
		// excessively during normal failures
		expectedTimeout := 10 * time.Second

		assert.Equal(t, expectedTimeout, publishConfirmTimeout,
			"publishConfirmTimeout should be 10s for optimal chaos test handling")
	})
}
