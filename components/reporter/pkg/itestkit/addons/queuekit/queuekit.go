// Package queuekit provides generic message queue consumption for E2E tests.
// It supports waiting for messages with filtering, automatic unmarshaling via generics,
// and multiple backend implementations (AMQP, etc.).
//
// This addon is decoupled from itestkit core and can be used independently.
package queuekit

import (
	"context"
	"encoding/json"
	"time"
)

// Message represents a raw message from the queue with metadata.
type Message struct {
	// Body is the raw message payload.
	Body []byte

	// Headers contains message headers/properties.
	Headers map[string]any

	// RoutingKey is the routing key (for AMQP) or equivalent.
	RoutingKey string

	// Timestamp is when the message was published (if available).
	Timestamp time.Time

	// MessageID is the unique message identifier (if available).
	MessageID string

	// CorrelationID is the correlation identifier (if available).
	CorrelationID string

	// ContentType is the MIME type of the message body.
	ContentType string
}

// ParsedMessage wraps a Message with its parsed payload.
type ParsedMessage[T any] struct {
	Message
	Payload T
}

// Unmarshaler defines how to unmarshal message bodies.
type Unmarshaler func(data []byte, v any) error

// DefaultUnmarshaler uses JSON unmarshaling.
func DefaultUnmarshaler() Unmarshaler {
	return json.Unmarshal
}

// QueueConsumer defines the interface for consuming messages from a queue.
// Implementations must be safe for concurrent use.
type QueueConsumer interface {
	// Consume starts consuming messages and returns a channel.
	// The channel is closed when the context is canceled or an error occurs.
	Consume(ctx context.Context) (<-chan Message, error)

	// Close releases all resources.
	Close() error
}

// QueuePublisher defines the interface for publishing messages to a queue.
type QueuePublisher interface {
	// Publish sends a message to the specified destination.
	Publish(ctx context.Context, destination string, body []byte, opts ...PublishOption) error

	// Close releases all resources.
	Close() error
}

// PublishOptions contains options for publishing a message.
type PublishOptions struct {
	Headers       map[string]any
	RoutingKey    string
	ContentType   string
	CorrelationID string
	MessageID     string
	Persistent    bool
}

// PublishOption is a functional option for publishing.
type PublishOption func(*PublishOptions)

// WithHeaders sets message headers.
func WithHeaders(headers map[string]any) PublishOption {
	return func(o *PublishOptions) {
		o.Headers = headers
	}
}

// WithRoutingKey sets the routing key.
func WithRoutingKey(key string) PublishOption {
	return func(o *PublishOptions) {
		o.RoutingKey = key
	}
}

// WithContentType sets the content type.
func WithContentType(contentType string) PublishOption {
	return func(o *PublishOptions) {
		o.ContentType = contentType
	}
}

// WithCorrelationID sets the correlation ID.
func WithCorrelationID(id string) PublishOption {
	return func(o *PublishOptions) {
		o.CorrelationID = id
	}
}

// WithMessageID sets the message ID.
func WithMessageID(id string) PublishOption {
	return func(o *PublishOptions) {
		o.MessageID = id
	}
}

// WithPersistent marks the message as persistent.
func WithPersistent() PublishOption {
	return func(o *PublishOptions) {
		o.Persistent = true
	}
}

// applyPublishOptions applies all options to a PublishOptions struct.
func applyPublishOptions(opts []PublishOption) PublishOptions {
	po := PublishOptions{
		ContentType: "application/json",
	}
	for _, opt := range opts {
		opt(&po)
	}

	return po
}

// WaitResult contains the result of waiting for messages.
type WaitResult[T any] struct {
	// Messages contains all matched messages.
	Messages []ParsedMessage[T]

	// Unmatched contains messages that didn't match the filter.
	Unmatched []Message

	// Errors contains any parsing errors encountered.
	Errors []error

	// Duration is how long the wait took.
	Duration time.Duration

	// TimedOut indicates if the wait timed out.
	TimedOut bool
}

// First returns the first matched message, or the zero value if none.
func (r WaitResult[T]) First() (ParsedMessage[T], bool) {
	if len(r.Messages) == 0 {
		var zero ParsedMessage[T]
		return zero, false
	}

	return r.Messages[0], true
}

// Count returns the number of matched messages.
func (r WaitResult[T]) Count() int {
	return len(r.Messages)
}

// HasErrors returns true if any errors occurred during parsing.
func (r WaitResult[T]) HasErrors() bool {
	return len(r.Errors) > 0
}
