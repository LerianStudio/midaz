package queuekit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Consumer provides a fluent API for consuming and waiting for messages.
// It wraps a QueueConsumer and adds filtering, parsing, and wait functionality.
type Consumer[T any] struct {
	t           *testing.T
	backend     QueueConsumer
	matcher     Matcher
	unmarshaler Unmarshaler
	timeout     time.Duration
	debugLog    bool

	mu       sync.Mutex
	captured []Message
}

// ConsumerBuilder builds a Consumer with fluent configuration.
type ConsumerBuilder[T any] struct {
	t           *testing.T
	backend     QueueConsumer
	matcher     Matcher
	unmarshaler Unmarshaler
	timeout     time.Duration
	debugLog    bool
}

// NewConsumer creates a new ConsumerBuilder for type T.
func NewConsumer[T any](t *testing.T, backend QueueConsumer) *ConsumerBuilder[T] {
	t.Helper()

	return &ConsumerBuilder[T]{
		t:           t,
		backend:     backend,
		matcher:     MatchAlways(),
		unmarshaler: DefaultUnmarshaler(),
		timeout:     30 * time.Second,
		debugLog:    false,
	}
}

// WithMatcher sets the message matcher for filtering.
func (b *ConsumerBuilder[T]) WithMatcher(m Matcher) *ConsumerBuilder[T] {
	if m != nil {
		b.matcher = m
	}

	return b
}

// WithUnmarshaler sets a custom unmarshaler.
func (b *ConsumerBuilder[T]) WithUnmarshaler(u Unmarshaler) *ConsumerBuilder[T] {
	if u != nil {
		b.unmarshaler = u
	}

	return b
}

// WithTimeout sets the default timeout for wait operations.
func (b *ConsumerBuilder[T]) WithTimeout(d time.Duration) *ConsumerBuilder[T] {
	if d > 0 {
		b.timeout = d
	}

	return b
}

// WithDebugLog enables debug logging of received messages.
func (b *ConsumerBuilder[T]) WithDebugLog(enabled bool) *ConsumerBuilder[T] {
	b.debugLog = enabled
	return b
}

// Build creates the Consumer. Call Close() when done.
func (b *ConsumerBuilder[T]) Build() *Consumer[T] {
	b.t.Helper()

	return &Consumer[T]{
		t:           b.t,
		backend:     b.backend,
		matcher:     b.matcher,
		unmarshaler: b.unmarshaler,
		timeout:     b.timeout,
		debugLog:    b.debugLog,
		captured:    make([]Message, 0),
	}
}

// Close releases all resources.
func (c *Consumer[T]) Close() error {
	return c.backend.Close()
}

// WaitForMessage waits for a single message matching the filter.
// Returns an error if timeout is reached.
func (c *Consumer[T]) WaitForMessage(ctx context.Context) (ParsedMessage[T], error) {
	c.t.Helper()

	result, err := c.WaitForMessages(ctx, 1)
	if err != nil {
		var zero ParsedMessage[T]
		return zero, err
	}

	if len(result.Messages) == 0 {
		var zero ParsedMessage[T]
		return zero, fmt.Errorf("no matching messages received")
	}

	return result.Messages[0], nil
}

// WaitForMessages waits for n messages matching the filter.
// Returns a WaitResult containing all matched messages and metadata.
func (c *Consumer[T]) WaitForMessages(ctx context.Context, n int) (WaitResult[T], error) {
	c.t.Helper()

	if n <= 0 {
		return WaitResult[T]{}, fmt.Errorf("n must be > 0, got %d", n)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	start := time.Now()
	result := WaitResult[T]{
		Messages:  make([]ParsedMessage[T], 0, n),
		Unmatched: make([]Message, 0),
		Errors:    make([]error, 0),
	}

	msgs, err := c.backend.Consume(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to start consuming: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(start)
			result.TimedOut = ctx.Err() == context.DeadlineExceeded

			if result.TimedOut && len(result.Messages) == 0 {
				return result, fmt.Errorf("timeout waiting for messages: wanted %d, got %d", n, len(result.Messages))
			}

			return result, nil

		case msg, ok := <-msgs:
			if !ok {
				result.Duration = time.Since(start)
				if len(result.Messages) < n {
					return result, fmt.Errorf("channel closed: wanted %d messages, got %d", n, len(result.Messages))
				}

				return result, nil
			}

			c.logMessage(msg, "received")
			c.captureMessage(msg)

			if !c.matcher(msg) {
				result.Unmatched = append(result.Unmatched, msg)
				c.logMessage(msg, "unmatched")

				continue
			}

			var payload T
			if err := c.unmarshaler(msg.Body, &payload); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("unmarshal error: %w", err))
				c.logMessage(msg, "parse error: "+err.Error())

				continue
			}

			parsed := ParsedMessage[T]{
				Message: msg,
				Payload: payload,
			}
			result.Messages = append(result.Messages, parsed)

			c.logMessage(msg, "matched")

			if len(result.Messages) >= n {
				result.Duration = time.Since(start)
				return result, nil
			}
		}
	}
}

// CaptureAll captures all messages for a duration, then returns them.
// This is useful for debugging or verifying message flow.
func (c *Consumer[T]) CaptureAll(ctx context.Context, duration time.Duration) (WaitResult[T], error) {
	c.t.Helper()

	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	start := time.Now()
	result := WaitResult[T]{
		Messages:  make([]ParsedMessage[T], 0),
		Unmatched: make([]Message, 0),
		Errors:    make([]error, 0),
	}

	msgs, err := c.backend.Consume(ctx)
	if err != nil {
		return result, fmt.Errorf("failed to start consuming: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			result.Duration = time.Since(start)
			result.TimedOut = false // Expected timeout

			return result, nil

		case msg, ok := <-msgs:
			if !ok {
				result.Duration = time.Since(start)
				return result, nil
			}

			c.logMessage(msg, "captured")
			c.captureMessage(msg)

			if !c.matcher(msg) {
				result.Unmatched = append(result.Unmatched, msg)
				continue
			}

			var payload T
			if err := c.unmarshaler(msg.Body, &payload); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("unmarshal error: %w", err))
				continue
			}

			parsed := ParsedMessage[T]{
				Message: msg,
				Payload: payload,
			}
			result.Messages = append(result.Messages, parsed)
		}
	}
}

// AssertNoMessages asserts that no messages matching the filter are received
// within the timeout duration. Returns an error if any messages are received.
func (c *Consumer[T]) AssertNoMessages(ctx context.Context, duration time.Duration) error {
	c.t.Helper()

	result, err := c.CaptureAll(ctx, duration)
	if err != nil {
		return err
	}

	if len(result.Messages) > 0 {
		return fmt.Errorf("expected no messages, but received %d", len(result.Messages))
	}

	return nil
}

// DrainQueue consumes and discards all pending messages.
// Useful for cleaning up before a test.
func (c *Consumer[T]) DrainQueue(ctx context.Context, maxDuration time.Duration) (int, error) {
	c.t.Helper()

	ctx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()

	msgs, err := c.backend.Consume(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to start consuming: %w", err)
	}

	count := 0
	drainTimeout := time.After(100 * time.Millisecond) // Short timeout between messages

	for {
		select {
		case <-ctx.Done():
			return count, nil

		case msg, ok := <-msgs:
			if !ok {
				return count, nil
			}

			count++

			c.logMessage(msg, "drained")
			// Reset drain timeout
			drainTimeout = time.After(100 * time.Millisecond)

		case <-drainTimeout:
			// No more messages for 100ms, assume queue is empty
			return count, nil
		}
	}
}

// GetCaptured returns all captured messages (for debugging).
func (c *Consumer[T]) GetCaptured() []Message {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := make([]Message, len(c.captured))
	copy(result, c.captured)

	return result
}

// ClearCaptured clears the captured messages.
func (c *Consumer[T]) ClearCaptured() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.captured = make([]Message, 0)
}

// captureMessage stores a message for later inspection.
func (c *Consumer[T]) captureMessage(msg Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.captured = append(c.captured, msg)
}

// logMessage logs a message if debug logging is enabled.
func (c *Consumer[T]) logMessage(msg Message, action string) {
	if !c.debugLog {
		return
	}

	c.t.Logf("[queuekit] %s: routing_key=%q msg_id=%q body=%s",
		action, msg.RoutingKey, msg.MessageID, truncateBody(msg.Body, 200))
}

// truncateBody truncates a body for logging.
func truncateBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}

	return string(body[:maxLen]) + "..."
}

// WaitFor is a convenience function that creates a consumer, waits for a message,
// and closes the consumer. Use this for simple one-off waits.
func WaitFor[T any](
	ctx context.Context,
	t *testing.T,
	backend QueueConsumer,
	matcher Matcher,
	timeout time.Duration,
) (ParsedMessage[T], error) {
	t.Helper()

	consumer := NewConsumer[T](t, backend).
		WithMatcher(matcher).
		WithTimeout(timeout).
		Build()
	defer consumer.Close()

	return consumer.WaitForMessage(ctx)
}

// WaitForN is a convenience function that creates a consumer, waits for n messages,
// and closes the consumer.
func WaitForN[T any](
	ctx context.Context,
	t *testing.T,
	backend QueueConsumer,
	matcher Matcher,
	n int,
	timeout time.Duration,
) (WaitResult[T], error) {
	t.Helper()

	consumer := NewConsumer[T](t, backend).
		WithMatcher(matcher).
		WithTimeout(timeout).
		Build()
	defer consumer.Close()

	return consumer.WaitForMessages(ctx, n)
}
