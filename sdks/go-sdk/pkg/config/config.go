package config

import (
	"time"
)

// Default configuration values
const (
	// Default timeout for HTTP requests in seconds
	DefaultTimeout = 60

	// Default URLs for local Docker deployment
	DefaultOnboardingURL  = "http://localhost:3000"
	DefaultTransactionURL = "http://localhost:3001"

	// Default retry configuration
	DefaultMaxRetries   = 3
	DefaultRetryWaitMin = 1 * time.Second
	DefaultRetryWaitMax = 30 * time.Second
)

// Config holds the configuration for the Midaz client
type Config struct {
	// OnboardingURL is the base URL for the Onboarding API
	OnboardingURL string

	// TransactionURL is the base URL for the Transaction API
	TransactionURL string

	// AuthToken is the bearer token for authentication
	AuthToken string

	// Timeout is the timeout for HTTP requests in seconds
	Timeout time.Duration

	// UserAgent is the user agent to use for HTTP requests
	UserAgent string

	// MaxRetries is the maximum number of retries for failed requests
	MaxRetries int

	// RetryWaitMin is the minimum time to wait between retries
	RetryWaitMin time.Duration

	// RetryWaitMax is the maximum time to wait between retries
	RetryWaitMax time.Duration

	// Debug enables debug logging
	Debug bool
}

// Option is a function that can be passed to NewConfig to customize the configuration
type Option func(*Config)

// WithOnboardingURL sets the base URL for the Onboarding API
func WithOnboardingURL(url string) Option {
	return func(c *Config) {
		c.OnboardingURL = url
	}
}

// WithTransactionURL sets the base URL for the Transaction API
func WithTransactionURL(url string) Option {
	return func(c *Config) {
		c.TransactionURL = url
	}
}

// WithBaseURL sets both Onboarding and Transaction URLs to the same base URL
// with appropriate port differences
func WithBaseURL(baseURL string) Option {
	return func(c *Config) {
		c.OnboardingURL = baseURL + ":3000"

		c.TransactionURL = baseURL + ":3001"
	}
}

// WithAuthToken sets the bearer token for authentication
func WithAuthToken(token string) Option {
	return func(c *Config) {
		c.AuthToken = token
	}
}

// WithTimeout sets the timeout for HTTP requests in seconds
func WithTimeout(timeout int) Option {
	return func(c *Config) {
		c.Timeout = time.Duration(timeout) * time.Second
	}
}

// WithUserAgent sets the user agent for HTTP requests
func WithUserAgent(userAgent string) Option {
	return func(c *Config) {
		c.UserAgent = userAgent
	}
}

// WithMaxRetries sets the maximum number of retries for failed requests
func WithMaxRetries(maxRetries int) Option {
	return func(c *Config) {
		c.MaxRetries = maxRetries
	}
}

// WithRetryWaitMin sets the minimum time to wait between retries
func WithRetryWaitMin(retryWaitMin time.Duration) Option {
	return func(c *Config) {
		c.RetryWaitMin = retryWaitMin
	}
}

// WithRetryWaitMax sets the maximum time to wait between retries
func WithRetryWaitMax(retryWaitMax time.Duration) Option {
	return func(c *Config) {
		c.RetryWaitMax = retryWaitMax
	}
}

// WithDebug enables or disables debug logging
func WithDebug(debug bool) Option {
	return func(c *Config) {
		c.Debug = debug
	}
}

// NewConfig creates a new configuration with the provided options
func NewConfig(options ...Option) *Config {
	config := &Config{
		OnboardingURL:  DefaultOnboardingURL,
		TransactionURL: DefaultTransactionURL,
		Timeout:        DefaultTimeout * time.Second,
		UserAgent:      "midaz-go-sdk/v1",
		MaxRetries:     DefaultMaxRetries,
		RetryWaitMin:   DefaultRetryWaitMin,
		RetryWaitMax:   DefaultRetryWaitMax,
		Debug:          false,
	}

	// Apply options
	for _, option := range options {
		option(config)
	}

	return config
}

// NewLocalConfig creates a new configuration for local development
func NewLocalConfig(authToken string) *Config {
	return NewConfig(
		WithOnboardingURL(DefaultOnboardingURL),
		WithTransactionURL(DefaultTransactionURL),
		WithAuthToken(authToken),
	)
}
