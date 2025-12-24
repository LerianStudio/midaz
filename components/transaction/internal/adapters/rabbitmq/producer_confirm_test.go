package rabbitmq

import (
	"context"
	"testing"
	"time"

	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
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

// TestProducerDefaultWithConfirms_ChannelSetup validates the producer handles
// nil connection scenarios gracefully with proper error reporting.
// This is critical for chaos scenarios where the RabbitMQ connection may be
// disrupted and the connection becomes nil.
func TestProducerDefaultWithConfirms_ChannelSetup(t *testing.T) {
	t.Parallel()

	t.Run("returns error when connection is nil", func(t *testing.T) {
		t.Parallel()

		// Create a producer with nil connection
		// This simulates a complete connection loss scenario
		repo := &ProducerRabbitMQRepository{
			conn: nil, // Simulate completely disconnected state
		}

		ctx := context.Background()

		// Attempt to produce a message should fail gracefully
		_, err := repo.ProducerDefault(ctx, "test-exchange", "test-key", []byte(`{"test": "message"}`))

		// The producer should return an error, not panic
		assert.Error(t, err, "ProducerDefault should return error when connection is nil")
		// Check that the error chain contains the nil connection error
		assert.ErrorIs(t, err, ErrNilConnection, "error should indicate nil connection")
	})

	t.Run("respects context cancellation during publish with nil connection", func(t *testing.T) {
		t.Parallel()

		// Create a producer with nil connection
		repo := &ProducerRabbitMQRepository{
			conn: nil,
		}

		// Create a cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Attempt to produce should fail (nil connection check happens before context check)
		_, err := repo.ProducerDefault(ctx, "test-exchange", "test-key", []byte(`{"test": "message"}`))

		// Should return an error for nil connection
		assert.Error(t, err, "ProducerDefault should return error on nil connection")
	})

	t.Run("respects context cancellation with valid connection struct", func(t *testing.T) {
		t.Parallel()

		// Create a producer with a valid connection struct but no actual connection
		// The connection struct exists but internal state is not initialized
		repo := &ProducerRabbitMQRepository{
			conn: &libRabbitmq.RabbitMQConnection{},
		}

		// Create a cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Attempt to produce should detect context cancellation
		_, err := repo.ProducerDefault(ctx, "test-exchange", "test-key", []byte(`{"test": "message"}`))

		// Should return an error - either context cancelled or connection failure
		assert.Error(t, err, "ProducerDefault should return error")
		// Check that the error chain contains the context cancellation error
		assert.ErrorIs(t, err, context.Canceled, "error should indicate context cancellation")
	})
}

// TestProducerRepository_Interface validates that ProducerRabbitMQRepository
// properly implements the ProducerRepository interface.
func TestProducerRepository_Interface(t *testing.T) {
	t.Parallel()

	// Verify interface implementation at compile time
	var _ ProducerRepository = (*ProducerRabbitMQRepository)(nil)
}
