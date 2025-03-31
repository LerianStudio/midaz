package environment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	// Save original values to restore after test
	origClientID := ClientID
	origClientSecret := ClientSecret
	origURLAPIAuth := URLAPIAuth
	origURLAPILedger := URLAPILedger
	origURLAPITransaction := URLAPITransaction
	origVersion := Version

	// Restore original values after test
	defer func() {
		ClientID = origClientID
		ClientSecret = origClientSecret
		URLAPIAuth = origURLAPIAuth
		URLAPILedger = origURLAPILedger
		URLAPITransaction = origURLAPITransaction
		Version = origVersion
	}()

	// Set test values
	ClientID = "test-client-id"
	ClientSecret = "test-client-secret"
	URLAPIAuth = "https://auth.example.com"
	URLAPILedger = "https://ledger.example.com"
	URLAPITransaction = "https://transaction.example.com"
	Version = "1.0.0"

	// Create new environment
	env := New()

	// Verify values
	assert.Equal(t, "test-client-id", env.ClientID)
	assert.Equal(t, "test-client-secret", env.ClientSecret)
	assert.Equal(t, "https://auth.example.com", env.URLAPIAuth)
	assert.Equal(t, "https://ledger.example.com", env.URLAPILedger)
	assert.Equal(t, "https://transaction.example.com", env.URLAPITransaction)
	assert.Equal(t, CLIVersion+"1.0.0", env.Version)
}

func TestEnvStructInitialization(t *testing.T) {
	// Save original values to restore after test
	origClientID := ClientID
	origClientSecret := ClientSecret
	origURLAPIAuth := URLAPIAuth
	origURLAPILedger := URLAPILedger
	origURLAPITransaction := URLAPITransaction
	origVersion := Version

	// Restore original values after test
	defer func() {
		ClientID = origClientID
		ClientSecret = origClientSecret
		URLAPIAuth = origURLAPIAuth
		URLAPILedger = origURLAPILedger
		URLAPITransaction = origURLAPITransaction
		Version = origVersion
	}()

	// Test with empty values
	ClientID = ""
	ClientSecret = ""
	URLAPIAuth = ""
	URLAPILedger = ""
	URLAPITransaction = ""
	Version = ""

	// Create new environment
	env := New()

	// Verify values
	assert.Equal(t, "", env.ClientID)
	assert.Equal(t, "", env.ClientSecret)
	assert.Equal(t, "", env.URLAPIAuth)
	assert.Equal(t, "", env.URLAPILedger)
	assert.Equal(t, "", env.URLAPITransaction)
	assert.Equal(t, CLIVersion, env.Version)

	// Test with non-empty values
	ClientID = "client-id"
	ClientSecret = "client-secret"
	URLAPIAuth = "auth-url"
	URLAPILedger = "ledger-url"
	URLAPITransaction = "transaction-url"
	Version = "2.0.0"

	// Create new environment
	env = New()

	// Verify values
	assert.Equal(t, "client-id", env.ClientID)
	assert.Equal(t, "client-secret", env.ClientSecret)
	assert.Equal(t, "auth-url", env.URLAPIAuth)
	assert.Equal(t, "ledger-url", env.URLAPILedger)
	assert.Equal(t, "transaction-url", env.URLAPITransaction)
	assert.Equal(t, CLIVersion+"2.0.0", env.Version)
}
