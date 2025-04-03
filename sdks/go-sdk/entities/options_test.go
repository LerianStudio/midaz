package entities

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptions(t *testing.T) {
	t.Run("WithDebug", func(t *testing.T) {
		// Create a new entity
		entity := &Entity{
			httpClient: &httpClient{
				debug: false,
			},
		}

		// Apply the WithDebug option
		opt := WithDebug(true)
		err := opt(entity)

		// Check that there was no error
		assert.NoError(t, err)

		// Check that the debug flag was set
		assert.True(t, entity.httpClient.debug)

		// Test setting it back to false
		opt = WithDebug(false)
		err = opt(entity)

		// Check that there was no error
		assert.NoError(t, err)

		// Check that the debug flag was unset
		assert.False(t, entity.httpClient.debug)
	})

	t.Run("WithUserAgent", func(t *testing.T) {
		// Create a new entity
		entity := &Entity{
			httpClient: &httpClient{
				userAgent: "default-agent",
			},
		}

		// Apply the WithUserAgent option
		opt := WithUserAgent("test-agent")
		err := opt(entity)

		// Check that there was no error
		assert.NoError(t, err)

		// Check that the user agent was set
		assert.Equal(t, "test-agent", entity.httpClient.userAgent)

		// Test setting it to a different value
		opt = WithUserAgent("another-agent")
		err = opt(entity)

		// Check that there was no error
		assert.NoError(t, err)

		// Check that the user agent was updated
		assert.Equal(t, "another-agent", entity.httpClient.userAgent)
	})
}
