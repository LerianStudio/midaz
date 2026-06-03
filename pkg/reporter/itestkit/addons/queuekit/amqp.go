package queuekit

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQPConfig contains configuration for AMQP consumer/publisher.
type AMQPConfig struct {
	// URL is the AMQP connection URL (e.g., amqp://guest:guest@localhost:5672/).
	URL string

	// Queue is the queue name to consume from.
	Queue string

	// Exchange is the exchange to bind to (optional, for dynamic binding).
	Exchange string

	// BindingKey is the routing key pattern for binding (optional).
	BindingKey string

	// AutoAck determines if messages are automatically acknowledged.
	AutoAck bool

	// Exclusive makes the consumer exclusive to this connection.
	Exclusive bool

	// PrefetchCount limits the number of unacknowledged messages.
	PrefetchCount int

	// DeclareQueue creates the queue if it doesn't exist.
	DeclareQueue bool

	// QueueDurable makes the queue durable (survives broker restart).
	QueueDurable bool

	// QueueAutoDelete deletes the queue when last consumer disconnects.
	QueueAutoDelete bool
}

// AMQPConsumer implements QueueConsumer for AMQP (RabbitMQ).
type AMQPConsumer struct {
	config AMQPConfig

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
	closed  bool
}

// NewAMQPConsumer creates a new AMQP consumer.
func NewAMQPConsumer(config AMQPConfig) (*AMQPConsumer, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("AMQP URL is required")
	}

	if config.Queue == "" {
		return nil, fmt.Errorf("queue name is required")
	}

	// Apply defaults
	if config.PrefetchCount == 0 {
		config.PrefetchCount = 10
	}

	return &AMQPConsumer{
		config: config,
	}, nil
}

// connect establishes connection and channel.
func (c *AMQPConsumer) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("consumer is closed")
	}

	if c.conn != nil && !c.conn.IsClosed() {
		return nil
	}

	conn, err := amqp.Dial(c.config.URL)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	if err := ch.Qos(c.config.PrefetchCount, 0, false); err != nil {
		_ = ch.Close()
		_ = conn.Close()

		return fmt.Errorf("failed to set QoS: %w", err)
	}

	// Optionally declare the queue
	if c.config.DeclareQueue {
		_, err := ch.QueueDeclare(
			c.config.Queue,
			c.config.QueueDurable,
			c.config.QueueAutoDelete,
			c.config.Exclusive,
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			_ = ch.Close()
			_ = conn.Close()

			return fmt.Errorf("failed to declare queue: %w", err)
		}
	}

	// Optionally bind the queue to an exchange
	if c.config.Exchange != "" && c.config.BindingKey != "" {
		err := ch.QueueBind(
			c.config.Queue,
			c.config.BindingKey,
			c.config.Exchange,
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			_ = ch.Close()
			_ = conn.Close()

			return fmt.Errorf("failed to bind queue: %w", err)
		}
	}

	c.conn = conn
	c.channel = ch

	return nil
}

// Consume starts consuming messages and returns a channel.
func (c *AMQPConsumer) Consume(ctx context.Context) (<-chan Message, error) {
	if err := c.connect(); err != nil {
		return nil, err
	}

	c.mu.Lock()
	ch := c.channel
	c.mu.Unlock()

	deliveries, err := ch.Consume(
		c.config.Queue,
		"",                 // consumer tag (auto-generated)
		c.config.AutoAck,   // auto-ack
		c.config.Exclusive, // exclusive
		false,              // no-local
		false,              // no-wait
		nil,                // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start consuming: %w", err)
	}

	out := make(chan Message)

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-deliveries:
				if !ok {
					return
				}

				msg := deliveryToMessage(d)

				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// Close releases all resources.
func (c *AMQPConsumer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true

	var errs []error

	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			errs = append(errs, err)
		}

		c.channel = nil
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			errs = append(errs, err)
		}

		c.conn = nil
	}

	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

// deliveryToMessage converts an AMQP delivery to a Message.
func deliveryToMessage(d amqp.Delivery) Message {
	headers := make(map[string]any)
	for k, v := range d.Headers {
		headers[k] = v
	}

	return Message{
		Body:          d.Body,
		Headers:       headers,
		RoutingKey:    d.RoutingKey,
		Timestamp:     d.Timestamp,
		MessageID:     d.MessageId,
		CorrelationID: d.CorrelationId,
		ContentType:   d.ContentType,
	}
}

// AMQPPublisher implements QueuePublisher for AMQP (RabbitMQ).
type AMQPPublisher struct {
	url string

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
	closed  bool
}

// NewAMQPPublisher creates a new AMQP publisher.
func NewAMQPPublisher(url string) (*AMQPPublisher, error) {
	if url == "" {
		return nil, fmt.Errorf("AMQP URL is required")
	}

	return &AMQPPublisher{
		url: url,
	}, nil
}

// connect establishes connection and channel.
func (p *AMQPPublisher) connect() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("publisher is closed")
	}

	if p.conn != nil && !p.conn.IsClosed() {
		return nil
	}

	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}

	p.conn = conn
	p.channel = ch

	return nil
}

// Publish sends a message to the specified exchange.
func (p *AMQPPublisher) Publish(ctx context.Context, exchange string, body []byte, opts ...PublishOption) error {
	if err := p.connect(); err != nil {
		return err
	}

	po := applyPublishOptions(opts)

	headers := amqp.Table{}
	for k, v := range po.Headers {
		headers[k] = v
	}

	deliveryMode := amqp.Transient
	if po.Persistent {
		deliveryMode = amqp.Persistent
	}

	publishing := amqp.Publishing{
		Headers:       headers,
		ContentType:   po.ContentType,
		DeliveryMode:  deliveryMode,
		CorrelationId: po.CorrelationID,
		MessageId:     po.MessageID,
		Timestamp:     time.Now(),
		Body:          body,
	}

	p.mu.Lock()

	ch := p.channel
	if ch == nil {
		p.mu.Unlock()
		return fmt.Errorf("publisher channel is nil")
	}

	err := ch.PublishWithContext(
		ctx,
		exchange,
		po.RoutingKey,
		false, // mandatory
		false, // immediate
		publishing,
	)

	p.mu.Unlock()

	return err
}

// Close releases all resources.
func (p *AMQPPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true

	var errs []error

	if p.channel != nil {
		if err := p.channel.Close(); err != nil {
			errs = append(errs, err)
		}

		p.channel = nil
	}

	if p.conn != nil {
		if err := p.conn.Close(); err != nil {
			errs = append(errs, err)
		}

		p.conn = nil
	}

	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

// Compile-time interface verification
var (
	_ QueueConsumer  = (*AMQPConsumer)(nil)
	_ QueuePublisher = (*AMQPPublisher)(nil)
)

// AMQPConsumerBuilder provides a fluent API for building AMQP consumers.
type AMQPConsumerBuilder struct {
	config AMQPConfig
}

// NewAMQPConsumerBuilder creates a builder for AMQP consumer configuration.
func NewAMQPConsumerBuilder(url string) *AMQPConsumerBuilder {
	return &AMQPConsumerBuilder{
		config: AMQPConfig{
			URL:           url,
			AutoAck:       true,
			PrefetchCount: 10,
		},
	}
}

// FromQueue sets the queue name to consume from.
func (b *AMQPConsumerBuilder) FromQueue(queue string) *AMQPConsumerBuilder {
	b.config.Queue = queue
	return b
}

// BindTo binds the queue to an exchange with the given routing key.
func (b *AMQPConsumerBuilder) BindTo(exchange, bindingKey string) *AMQPConsumerBuilder {
	b.config.Exchange = exchange
	b.config.BindingKey = bindingKey

	return b
}

// WithAutoAck sets whether messages are automatically acknowledged.
func (b *AMQPConsumerBuilder) WithAutoAck(autoAck bool) *AMQPConsumerBuilder {
	b.config.AutoAck = autoAck
	return b
}

// WithExclusive makes the consumer exclusive.
func (b *AMQPConsumerBuilder) WithExclusive(exclusive bool) *AMQPConsumerBuilder {
	b.config.Exclusive = exclusive
	return b
}

// WithPrefetch sets the prefetch count.
func (b *AMQPConsumerBuilder) WithPrefetch(count int) *AMQPConsumerBuilder {
	if count > 0 {
		b.config.PrefetchCount = count
	}

	return b
}

// WithQueueDeclare enables automatic queue declaration.
func (b *AMQPConsumerBuilder) WithQueueDeclare(durable, autoDelete bool) *AMQPConsumerBuilder {
	b.config.DeclareQueue = true
	b.config.QueueDurable = durable
	b.config.QueueAutoDelete = autoDelete

	return b
}

// Build creates the AMQPConsumer.
func (b *AMQPConsumerBuilder) Build() (*AMQPConsumer, error) {
	return NewAMQPConsumer(b.config)
}
