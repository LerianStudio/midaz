package factory

import (
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/pkg/environment"
	"github.com/stretchr/testify/assert"
)

func TestNewFactory(t *testing.T) {
	// Create a test environment
	env := &environment.Env{
		ClientID:          "test-client-id",
		ClientSecret:      "test-client-secret",
		URLAPIAuth:        "https://auth.example.com",
		URLAPILedger:      "https://ledger.example.com",
		URLAPITransaction: "https://transaction.example.com",
		Version:           "1.0.0",
	}

	// Create a new factory
	f := NewFactory(env)

	// Verify the factory is created correctly
	assert.NotNil(t, f)
	assert.NotNil(t, f.HTTPClient)
	assert.NotNil(t, f.IOStreams)
	assert.Equal(t, env, f.Env)
	assert.Equal(t, "", f.Token)
	assert.False(t, f.Flags.NoColor)
}

func TestFactoryWithToken(t *testing.T) {
	// Create a test environment
	env := &environment.Env{
		ClientID:          "test-client-id",
		ClientSecret:      "test-client-secret",
		URLAPIAuth:        "https://auth.example.com",
		URLAPILedger:      "https://ledger.example.com",
		URLAPITransaction: "https://transaction.example.com",
		Version:           "1.0.0",
	}

	// Create a new factory
	f := NewFactory(env)

	// Set a token
	f.Token = "test-token"

	// Verify the token is set correctly
	assert.Equal(t, "test-token", f.Token)
}

func TestFactoryWithNoColorFlag(t *testing.T) {
	// Create a test environment
	env := &environment.Env{
		ClientID:          "test-client-id",
		ClientSecret:      "test-client-secret",
		URLAPIAuth:        "https://auth.example.com",
		URLAPILedger:      "https://ledger.example.com",
		URLAPITransaction: "https://transaction.example.com",
		Version:           "1.0.0",
	}

	// Create a new factory
	f := NewFactory(env)

	// Set the NoColor flag
	f.Flags.NoColor = true

	// Verify the NoColor flag is set correctly
	assert.True(t, f.Flags.NoColor)
}

func TestFactoryWithNilEnvironment(t *testing.T) {
	// Create a new factory with nil environment
	f := NewFactory(nil)

	// Verify the factory is created correctly
	assert.NotNil(t, f)
	assert.NotNil(t, f.HTTPClient)
	assert.NotNil(t, f.IOStreams)
	assert.Nil(t, f.Env)
}

func TestFactoryStructInitialization(t *testing.T) {
	// Test direct initialization of Factory struct
	f := &Factory{
		Token:      "direct-token",
		HTTPClient: nil,
		IOStreams:  nil,
		Env:        nil,
		Flags: Flags{
			NoColor: true,
		},
	}

	// Verify the factory is initialized correctly
	assert.Equal(t, "direct-token", f.Token)
	assert.Nil(t, f.HTTPClient)
	assert.Nil(t, f.IOStreams)
	assert.Nil(t, f.Env)
	assert.True(t, f.Flags.NoColor)
}
