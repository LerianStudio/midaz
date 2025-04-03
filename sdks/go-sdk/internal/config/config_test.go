package config

import (
	"testing"
	"time"
)

func TestDefaultValues(t *testing.T) {
	// Test that the default constants have the expected values
	if DefaultTimeout != 60 {
		t.Errorf("Expected DefaultTimeout to be 60, got %d", DefaultTimeout)
	}

	if DefaultOnboardingURL != "http://localhost:3000" {
		t.Errorf("Expected DefaultOnboardingURL to be http://localhost:3000, got %s", DefaultOnboardingURL)
	}

	if DefaultTransactionURL != "http://localhost:3001" {
		t.Errorf("Expected DefaultTransactionURL to be http://localhost:3001, got %s", DefaultTransactionURL)
	}

	if DefaultMaxRetries != 3 {
		t.Errorf("Expected DefaultMaxRetries to be 3, got %d", DefaultMaxRetries)
	}

	if DefaultRetryWaitMin != 1*time.Second {
		t.Errorf("Expected DefaultRetryWaitMin to be 1s, got %s", DefaultRetryWaitMin)
	}

	if DefaultRetryWaitMax != 30*time.Second {
		t.Errorf("Expected DefaultRetryWaitMax to be 30s, got %s", DefaultRetryWaitMax)
	}
}

func TestNewConfig(t *testing.T) {
	// Test creating a new config with default values
	config := NewConfig()

	// Check that default values are set correctly
	if config.OnboardingURL != DefaultOnboardingURL {
		t.Errorf("Expected OnboardingURL to be %s, got %s", DefaultOnboardingURL, config.OnboardingURL)
	}

	if config.TransactionURL != DefaultTransactionURL {
		t.Errorf("Expected TransactionURL to be %s, got %s", DefaultTransactionURL, config.TransactionURL)
	}

	if config.Timeout != DefaultTimeout*time.Second {
		t.Errorf("Expected Timeout to be %s, got %s", DefaultTimeout*time.Second, config.Timeout)
	}

	if config.UserAgent != "midaz-go-sdk/v1" {
		t.Errorf("Expected UserAgent to be midaz-go-sdk/v1, got %s", config.UserAgent)
	}

	if config.MaxRetries != DefaultMaxRetries {
		t.Errorf("Expected MaxRetries to be %d, got %d", DefaultMaxRetries, config.MaxRetries)
	}

	if config.RetryWaitMin != DefaultRetryWaitMin {
		t.Errorf("Expected RetryWaitMin to be %s, got %s", DefaultRetryWaitMin, config.RetryWaitMin)
	}

	if config.RetryWaitMax != DefaultRetryWaitMax {
		t.Errorf("Expected RetryWaitMax to be %s, got %s", DefaultRetryWaitMax, config.RetryWaitMax)
	}

	if config.Debug != false {
		t.Errorf("Expected Debug to be false, got %t", config.Debug)
	}
}

func TestWithOnboardingURL(t *testing.T) {
	// Test setting a custom onboarding URL
	customURL := "https://api.example.com/onboarding"
	config := NewConfig(WithOnboardingURL(customURL))

	if config.OnboardingURL != customURL {
		t.Errorf("Expected OnboardingURL to be %s, got %s", customURL, config.OnboardingURL)
	}
}

func TestWithTransactionURL(t *testing.T) {
	// Test setting a custom transaction URL
	customURL := "https://api.example.com/transaction"
	config := NewConfig(WithTransactionURL(customURL))

	if config.TransactionURL != customURL {
		t.Errorf("Expected TransactionURL to be %s, got %s", customURL, config.TransactionURL)
	}
}

func TestWithBaseURL(t *testing.T) {
	// Test setting a base URL that affects both onboarding and transaction URLs
	baseURL := "https://api.example.com"
	config := NewConfig(WithBaseURL(baseURL))

	expectedOnboardingURL := baseURL + ":3000"
	expectedTransactionURL := baseURL + ":3001"

	if config.OnboardingURL != expectedOnboardingURL {
		t.Errorf("Expected OnboardingURL to be %s, got %s", expectedOnboardingURL, config.OnboardingURL)
	}

	if config.TransactionURL != expectedTransactionURL {
		t.Errorf("Expected TransactionURL to be %s, got %s", expectedTransactionURL, config.TransactionURL)
	}
}

func TestWithAuthToken(t *testing.T) {
	// Test setting an auth token
	token := "test-auth-token"
	config := NewConfig(WithAuthToken(token))

	if config.AuthToken != token {
		t.Errorf("Expected AuthToken to be %s, got %s", token, config.AuthToken)
	}
}

func TestWithTimeout(t *testing.T) {
	// Test setting a custom timeout
	timeout := 30
	config := NewConfig(WithTimeout(timeout))

	expected := time.Duration(timeout) * time.Second
	if config.Timeout != expected {
		t.Errorf("Expected Timeout to be %s, got %s", expected, config.Timeout)
	}
}

func TestWithUserAgent(t *testing.T) {
	// Test setting a custom user agent
	userAgent := "custom-user-agent/1.0"
	config := NewConfig(WithUserAgent(userAgent))

	if config.UserAgent != userAgent {
		t.Errorf("Expected UserAgent to be %s, got %s", userAgent, config.UserAgent)
	}
}

func TestWithMaxRetries(t *testing.T) {
	// Test setting a custom max retries value
	maxRetries := 5
	config := NewConfig(WithMaxRetries(maxRetries))

	if config.MaxRetries != maxRetries {
		t.Errorf("Expected MaxRetries to be %d, got %d", maxRetries, config.MaxRetries)
	}
}

func TestWithRetryWaitMin(t *testing.T) {
	// Test setting a custom minimum retry wait time
	retryWaitMin := 2 * time.Second
	config := NewConfig(WithRetryWaitMin(retryWaitMin))

	if config.RetryWaitMin != retryWaitMin {
		t.Errorf("Expected RetryWaitMin to be %s, got %s", retryWaitMin, config.RetryWaitMin)
	}
}

func TestWithRetryWaitMax(t *testing.T) {
	// Test setting a custom maximum retry wait time
	retryWaitMax := 60 * time.Second
	config := NewConfig(WithRetryWaitMax(retryWaitMax))

	if config.RetryWaitMax != retryWaitMax {
		t.Errorf("Expected RetryWaitMax to be %s, got %s", retryWaitMax, config.RetryWaitMax)
	}
}

func TestWithDebug(t *testing.T) {
	// Test enabling debug mode
	config := NewConfig(WithDebug(true))

	if !config.Debug {
		t.Errorf("Expected Debug to be true, got false")
	}

	// Test disabling debug mode
	config = NewConfig(WithDebug(false))

	if config.Debug {
		t.Errorf("Expected Debug to be false, got true")
	}
}

func TestNewLocalConfig(t *testing.T) {
	// Test creating a local configuration
	token := "test-local-token"
	config := NewLocalConfig(token)

	// Check that local config values are set correctly
	if config.OnboardingURL != DefaultOnboardingURL {
		t.Errorf("Expected OnboardingURL to be %s, got %s", DefaultOnboardingURL, config.OnboardingURL)
	}

	if config.TransactionURL != DefaultTransactionURL {
		t.Errorf("Expected TransactionURL to be %s, got %s", DefaultTransactionURL, config.TransactionURL)
	}

	if config.AuthToken != token {
		t.Errorf("Expected AuthToken to be %s, got %s", token, config.AuthToken)
	}
}

func TestMultipleOptions(t *testing.T) {
	// Test applying multiple options at once
	config := NewConfig(
		WithOnboardingURL("https://api.example.com/onboarding"),
		WithTransactionURL("https://api.example.com/transaction"),
		WithAuthToken("test-token"),
		WithTimeout(30),
		WithUserAgent("custom-agent/1.0"),
		WithMaxRetries(5),
		WithRetryWaitMin(2*time.Second),
		WithRetryWaitMax(60*time.Second),
		WithDebug(true),
	)

	// Check that all options were applied correctly
	if config.OnboardingURL != "https://api.example.com/onboarding" {
		t.Errorf("Expected OnboardingURL to be https://api.example.com/onboarding, got %s", config.OnboardingURL)
	}

	if config.TransactionURL != "https://api.example.com/transaction" {
		t.Errorf("Expected TransactionURL to be https://api.example.com/transaction, got %s", config.TransactionURL)
	}

	if config.AuthToken != "test-token" {
		t.Errorf("Expected AuthToken to be test-token, got %s", config.AuthToken)
	}

	if config.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout to be 30s, got %s", config.Timeout)
	}

	if config.UserAgent != "custom-agent/1.0" {
		t.Errorf("Expected UserAgent to be custom-agent/1.0, got %s", config.UserAgent)
	}

	if config.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries to be 5, got %d", config.MaxRetries)
	}

	if config.RetryWaitMin != 2*time.Second {
		t.Errorf("Expected RetryWaitMin to be 2s, got %s", config.RetryWaitMin)
	}

	if config.RetryWaitMax != 60*time.Second {
		t.Errorf("Expected RetryWaitMax to be 60s, got %s", config.RetryWaitMax)
	}

	if !config.Debug {
		t.Errorf("Expected Debug to be true, got false")
	}
}

func TestOptionOverrides(t *testing.T) {
	// Test that later options override earlier ones
	config := NewConfig(
		WithOnboardingURL("https://api1.example.com"),
		WithOnboardingURL("https://api2.example.com"),
	)

	if config.OnboardingURL != "https://api2.example.com" {
		t.Errorf("Expected OnboardingURL to be https://api2.example.com, got %s", config.OnboardingURL)
	}

	// Test overriding with base URL
	config = NewConfig(
		WithOnboardingURL("https://api.example.com/onboarding"),
		WithBaseURL("https://base.example.com"),
	)

	if config.OnboardingURL != "https://base.example.com:3000" {
		t.Errorf("Expected OnboardingURL to be https://base.example.com:3000, got %s", config.OnboardingURL)
	}

	if config.TransactionURL != "https://base.example.com:3001" {
		t.Errorf("Expected TransactionURL to be https://base.example.com:3001, got %s", config.TransactionURL)
	}
}
