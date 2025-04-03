package entities

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConstants(t *testing.T) {
	t.Run("ServiceTypes", func(t *testing.T) {
		// Test that the service type constants have the expected values
		assert.Equal(t, "onboarding", ServiceOnboarding)
		assert.Equal(t, "transaction", ServiceTransaction)

		// Test that the constants are used correctly in a typical scenario
		baseURLs := map[string]string{
			ServiceOnboarding:  "https://api.example.com/onboarding",
			ServiceTransaction: "https://api.example.com/transaction",
		}

		assert.Equal(t, "https://api.example.com/onboarding", baseURLs[ServiceOnboarding])
		assert.Equal(t, "https://api.example.com/transaction", baseURLs[ServiceTransaction])
	})
}
