# Package utils

## Overview

The `utils` package provides utility functions and helpers used across the Midaz ledger system. Currently, it focuses on retry logic and exponential backoff with jitter for handling transient failures in distributed systems.

## Purpose

This package provides reusable utilities for:

- **Exponential backoff**: Gradually increasing delays between retry attempts
- **Jitter**: Randomizing retry delays to prevent thundering herd problems
- **Retry configuration**: Standard constants for retry behavior across the system

## Package Structure

```
utils/
├── jitter.go    # Exponential backoff and jitter utilities
└── README.md    # This file
```

## Key Components

### Retry Configuration Constants

The package defines standard retry behavior constants:

- **MaxRetries** (5): Maximum number of retry attempts before giving up
- **InitialBackoff** (500ms): Starting delay for the first retry
- **MaxBackoff** (10s): Maximum delay cap to prevent indefinite growth
- **BackoffFactor** (2.0): Exponential growth multiplier

### Functions

#### FullJitter

```go
func FullJitter(baseDelay time.Duration) time.Duration
```

Returns a random delay between [0, baseDelay], capped by MaxBackoff.

**Purpose**: Implements the "Full Jitter" strategy to prevent thundering herd problems where many clients retry simultaneously.

**Parameters**:

- `baseDelay`: The maximum delay duration for this retry attempt

**Returns**:

- A random duration between 0 and baseDelay, capped at MaxBackoff

**Example**:

```go
delay := utils.FullJitter(2 * time.Second)
time.Sleep(delay)
```

#### NextBackoff

```go
func NextBackoff(current time.Duration) time.Duration
```

Calculates the next exponential backoff delay.

**Purpose**: Implements exponential backoff by multiplying the current delay by BackoffFactor.

**Parameters**:

- `current`: The current delay duration

**Returns**:

- The next delay duration (current \* BackoffFactor), capped at MaxBackoff

**Example**:

```go
backoff := utils.NextBackoff(1 * time.Second)
// Returns 2 seconds (1s * 2.0)
```

## Usage Examples

### Basic Retry Pattern

```go
import (
    "time"
    "github.com/LerianStudio/midaz/v3/pkg/utils"
)

func connectWithRetry() error {
    backoff := utils.InitialBackoff

    for attempt := 0; attempt < utils.MaxRetries; attempt++ {
        if err := connect(); err != nil {
            if attempt < utils.MaxRetries-1 {
                // Sleep with jitter
                time.Sleep(utils.FullJitter(backoff))
                // Calculate next backoff
                backoff = utils.NextBackoff(backoff)
                continue
            }
            return err // Final attempt failed
        }
        return nil // Success
    }

    return errors.New("max retries exceeded")
}
```

### Database Connection Retry

```go
func connectToDatabase() (*sql.DB, error) {
    var db *sql.DB
    var err error
    backoff := utils.InitialBackoff

    for attempt := 0; attempt < utils.MaxRetries; attempt++ {
        db, err = sql.Open("postgres", connectionString)
        if err == nil {
            // Test the connection
            if err = db.Ping(); err == nil {
                return db, nil
            }
        }

        if attempt < utils.MaxRetries-1 {
            log.Printf("Database connection failed (attempt %d/%d): %v",
                attempt+1, utils.MaxRetries, err)
            time.Sleep(utils.FullJitter(backoff))
            backoff = utils.NextBackoff(backoff)
        }
    }

    return nil, fmt.Errorf("failed to connect after %d attempts: %w",
        utils.MaxRetries, err)
}
```

### Message Broker Retry

```go
func publishWithRetry(msg []byte) error {
    backoff := utils.InitialBackoff

    for attempt := 0; attempt < utils.MaxRetries; attempt++ {
        err := messageQueue.Publish(msg)
        if err == nil {
            return nil
        }

        // Check if error is retryable
        if !isRetryable(err) {
            return err
        }

        if attempt < utils.MaxRetries-1 {
            time.Sleep(utils.FullJitter(backoff))
            backoff = utils.NextBackoff(backoff)
        }
    }

    return constant.ErrMessageBrokerUnavailable
}
```

### HTTP Request Retry

```go
func fetchWithRetry(url string) (*http.Response, error) {
    backoff := utils.InitialBackoff

    for attempt := 0; attempt < utils.MaxRetries; attempt++ {
        resp, err := http.Get(url)

        // Success case
        if err == nil && resp.StatusCode < 500 {
            return resp, nil
        }

        // Close response body if present
        if resp != nil {
            resp.Body.Close()
        }

        // Retry on network errors or 5xx status codes
        if attempt < utils.MaxRetries-1 {
            time.Sleep(utils.FullJitter(backoff))
            backoff = utils.NextBackoff(backoff)
            continue
        }

        return nil, err
    }

    return nil, errors.New("max retries exceeded")
}
```

## Backoff Sequence

With the default configuration, the backoff sequence follows this pattern:

| Attempt | Base Delay | Jitter Range | Max Possible Delay |
| ------- | ---------- | ------------ | ------------------ |
| 1       | 500ms      | 0-500ms      | 500ms              |
| 2       | 1s         | 0-1s         | 1s                 |
| 3       | 2s         | 0-2s         | 2s                 |
| 4       | 4s         | 0-4s         | 4s                 |
| 5       | 8s         | 0-8s         | 8s                 |
| 6+      | 10s (cap)  | 0-10s        | 10s                |

The actual delay for each attempt is randomly chosen from the jitter range, providing better distribution of retry attempts.

## Design Principles

1. **Exponential Growth**: Delays grow exponentially to give systems time to recover
2. **Capped Backoff**: MaxBackoff prevents indefinite delay growth
3. **Full Jitter**: Randomization prevents synchronized retry storms
4. **Configurable**: Constants can be referenced for consistent behavior
5. **Simple API**: Two functions cover most retry scenarios

## When to Use

Use these utilities when dealing with:

- **Transient failures**: Network timeouts, temporary unavailability
- **External services**: Database connections, message brokers, APIs
- **Distributed systems**: Any operation that might temporarily fail
- **Rate limiting**: Backing off when hitting rate limits

## When NOT to Use

Avoid using these utilities for:

- **Permanent failures**: Invalid credentials, missing resources
- **User errors**: Bad input, validation failures
- **Business logic errors**: Insufficient funds, duplicate entries
- **Fast operations**: Operations that should fail immediately

## Best Practices

1. **Check for retryable errors**: Not all errors should trigger retries
2. **Log retry attempts**: Help with debugging and monitoring
3. **Set timeouts**: Combine with context deadlines to prevent infinite retries
4. **Use contexts**: Respect cancellation signals
5. **Monitor metrics**: Track retry rates to identify systemic issues

## Performance Considerations

- **Non-cryptographic randomness**: Uses `math/rand` for performance (sufficient for jitter)
- **Minimal allocations**: Simple calculations with no heap allocations
- **Lightweight**: No external dependencies beyond standard library

## Security Notes

The `#nosec G404` directive is used to suppress gosec warnings about using `math/rand` instead of `crypto/rand`. This is intentional and safe because:

- Jitter doesn't require cryptographic randomness
- Performance is important for retry logic
- Predictable randomness doesn't create security vulnerabilities in this context

## Dependencies

This package depends only on the Go standard library:

- `math/rand`: For generating random jitter values
- `time`: For duration calculations

## Related Packages

This package is used by:

- Database connection pools in onboarding and transaction services
- Message broker clients (RabbitMQ)
- HTTP clients for external API calls
- Cache clients (Valkey/Redis)

## Future Enhancements

Potential additions to this package:

- Context-aware retry functions
- Configurable retry strategies (decorrelated jitter, equal jitter)
- Retry with circuit breaker pattern
- Metrics collection for retry attempts
- Retry budget implementation

## References

- [AWS Architecture Blog: Exponential Backoff And Jitter](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
- [Google Cloud: Retry Pattern](https://cloud.google.com/architecture/scalable-and-resilient-apps#retry_pattern)

## Version History

This package follows semantic versioning as part of the Midaz v3 module.
