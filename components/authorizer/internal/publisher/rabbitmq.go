// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package publisher

import (
	"context"
	"fmt"
	"sync"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQPublisher publishes authorizer-approved operations to RabbitMQ.
type RabbitMQPublisher struct {
	url    string
	logger libLog.Logger

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewRabbitMQPublisher(url string, logger libLog.Logger) (*RabbitMQPublisher, error) {
	if url == "" {
		return nil, fmt.Errorf("rabbitmq url cannot be empty")
	}

	p := &RabbitMQPublisher{
		url:    url,
		logger: logger,
	}

	if err := p.connect(); err != nil {
		return nil, err
	}

	return p, nil
}

func (p *RabbitMQPublisher) connect() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.connectLocked()
}

func (p *RabbitMQPublisher) connectLocked() error {
	if p.conn != nil && !p.conn.IsClosed() && p.channel != nil {
		return nil
	}

	if p.channel != nil {
		_ = p.channel.Close()
		p.channel = nil
	}

	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}

	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("dial rabbitmq: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}

	p.conn = conn
	p.channel = channel

	return nil
}

func (p *RabbitMQPublisher) Publish(ctx context.Context, message Message) error {
	if p == nil {
		return fmt.Errorf("rabbitmq publisher is nil")
	}

	if len(message.Payload) == 0 {
		return fmt.Errorf("message payload cannot be empty")
	}

	if err := p.connect(); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	headers := amqp.Table{}
	for key, value := range message.Headers {
		headers[key] = value
	}

	contentType := message.ContentType
	if contentType == "" {
		contentType = "application/msgpack"
	}

	if err := p.channel.PublishWithContext(
		ctx,
		message.Exchange,
		message.RoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  contentType,
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
			Body:         message.Payload,
		},
	); err != nil {
		if p.logger != nil {
			p.logger.Warnf("Authorizer publisher failed exchange=%s key=%s: %v", message.Exchange, message.RoutingKey, err)
		}

		_ = p.channel.Close()
		_ = p.conn.Close()
		p.channel = nil
		p.conn = nil

		return err
	}

	return nil
}

func (p *RabbitMQPublisher) Close() error {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error

	if p.channel != nil {
		if err := p.channel.Close(); err != nil {
			firstErr = err
		}
		p.channel = nil
	}

	if p.conn != nil {
		if err := p.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.conn = nil
	}

	return firstErr
}
