package utils

import "sync"

// This file contains test helpers that need to be exported for use by tests
// in other packages. These functions should NOT be used in production code.

// ResetConfigForTesting resets the configuration singleton for testing purposes.
// This function should ONLY be called in test code.
func ResetConfigForTesting() {
	configMu.Lock()
	defer configMu.Unlock()

	configOnce = sync.Once{}
	config = retryConfig{}
}
