# Config Package

The config package provides configuration utilities for the Midaz SDK, allowing you to customize how the SDK interacts with the Midaz API.

## Usage

Import the package in your Go code:

```go
import "github.com/LerianStudio/midaz/sdks/go-sdk/pkg/config"
```

## Configuration

### Config

The `Config` struct holds the configuration for the Midaz client:

```go
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
```

## Default Values

The package provides default configuration values:

```go
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
```

## Creating Configurations

### NewConfig

Creates a new configuration with the provided options:

```go
func NewConfig(options ...Option) *Config
```

### NewLocalConfig

Creates a new configuration for local development:

```go
func NewLocalConfig(authToken string) *Config
```

## Configuration Options

The package uses the functional options pattern to configure the SDK:

```go
func WithOnboardingURL(url string) Option
func WithTransactionURL(url string) Option
func WithBaseURL(baseURL string) Option
func WithAuthToken(token string) Option
func WithTimeout(timeout int) Option
func WithUserAgent(userAgent string) Option
func WithMaxRetries(maxRetries int) Option
func WithRetryWaitMin(retryWaitMin time.Duration) Option
func WithRetryWaitMax(retryWaitMax time.Duration) Option
func WithDebug(debug bool) Option
```

## Examples

### Creating a Configuration for Production

```go
// Create a configuration for production
config := config.NewConfig(
    config.WithBaseURL("https://api.midaz.com"),
    config.WithAuthToken("your-auth-token"),
    config.WithTimeout(30),
    config.WithMaxRetries(5),
)
```

### Creating a Configuration for Local Development

```go
// Create a configuration for local development
config := config.NewLocalConfig("your-auth-token")
```

## Best Practices

1. Use `NewConfig` with appropriate options for production environments
2. Use `WithBaseURL` to set both Onboarding and Transaction URLs to the same base URL
3. Use `WithTimeout` to set appropriate timeouts for your use case
4. Use `WithMaxRetries` and retry wait options for resilient API interactions
5. Use `WithDebug` during development to enable debug logging
