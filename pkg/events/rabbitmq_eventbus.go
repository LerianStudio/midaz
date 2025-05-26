package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/lib-commons/commons/rabbitmq"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/streadway/amqp"
)

// RabbitMQEventBus implements EventBus using RabbitMQ
type RabbitMQEventBus struct {
	client      *rabbitmq.RabbitMQ
	handlers    map[EventType][]EventHandler
	mu          sync.RWMutex
	exchange    string
	consumerTag string
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

// NewRabbitMQEventBus creates a new RabbitMQ-based event bus
func NewRabbitMQEventBus(client *rabbitmq.RabbitMQ, exchange string) (*RabbitMQEventBus, error) {
	if client == nil {
		return nil, errors.New("rabbitmq client is required")
	}

	return &RabbitMQEventBus{
		client:      client,
		handlers:    make(map[EventType][]EventHandler),
		exchange:    exchange,
		consumerTag: fmt.Sprintf("eventbus-%s", uuid.New().String()),
		stopChan:    make(chan struct{}),
	}, nil
}

// Publish sends an event to be processed
func (eb *RabbitMQEventBus) Publish(ctx context.Context, event DomainEvent) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "eventbus.publish")
	defer span.End()

	// Add event metadata
	span.SetAttributes(
		libOpentelemetry.Attribute("event.id", event.ID.String()),
		libOpentelemetry.Attribute("event.type", string(event.Type)),
		libOpentelemetry.Attribute("event.aggregate_id", event.AggregateID.String()),
		libOpentelemetry.Attribute("event.aggregate_type", event.AggregateType),
	)

	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal event", err)
		return errors.Wrap(err, "failed to marshal event")
	}

	// Create AMQP message
	msg := amqp.Publishing{
		ContentType:  "application/json",
		Body:         data,
		DeliveryMode: amqp.Persistent,
		Headers: amqp.Table{
			"event_id":        event.ID.String(),
			"event_type":      string(event.Type),
			"aggregate_id":    event.AggregateID.String(),
			"aggregate_type":  event.AggregateType,
			"organization_id": event.OrganizationID.String(),
		},
	}

	if event.LedgerID != nil {
		msg.Headers["ledger_id"] = event.LedgerID.String()
	}

	if event.CorrelationID != nil {
		msg.Headers["correlation_id"] = *event.CorrelationID
		msg.CorrelationId = *event.CorrelationID
	}

	// Routing key based on event type
	routingKey := string(event.Type)

	// Publish to exchange
	if err := eb.client.Channel.Publish(
		eb.exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		msg,
	); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to publish event", err)
		logger.Errorf("Failed to publish event %s: %v", event.ID, err)
		return errors.Wrap(err, "failed to publish event")
	}

	logger.Infof("Published event %s of type %s", event.ID, event.Type)
	return nil
}

// PublishBatch sends multiple events to be processed
func (eb *RabbitMQEventBus) PublishBatch(ctx context.Context, events []DomainEvent) error {
	for _, event := range events {
		if err := eb.Publish(ctx, event); err != nil {
			return errors.Wrapf(err, "failed to publish event %s", event.ID)
		}
	}
	return nil
}

// Subscribe registers a handler for specific event types
func (eb *RabbitMQEventBus) Subscribe(handler EventHandler, eventTypes ...EventType) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for _, eventType := range eventTypes {
		if handler.CanHandle(eventType) {
			eb.handlers[eventType] = append(eb.handlers[eventType], handler)
		}
	}

	return nil
}

// Unsubscribe removes a handler
func (eb *RabbitMQEventBus) Unsubscribe(handler EventHandler) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for eventType, handlers := range eb.handlers {
		var filtered []EventHandler
		for _, h := range handlers {
			if h != handler {
				filtered = append(filtered, h)
			}
		}
		eb.handlers[eventType] = filtered
	}

	return nil
}

// Start begins processing events
func (eb *RabbitMQEventBus) Start(ctx context.Context) error {
	logger := libCommons.NewLoggerFromContext(ctx)

	// Declare queue for this consumer
	queueName := fmt.Sprintf("eventbus.%s", eb.consumerTag)
	q, err := eb.client.Channel.QueueDeclare(
		queueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return errors.Wrap(err, "failed to declare queue")
	}

	// Bind queue to exchange for all registered event types
	eb.mu.RLock()
	for eventType := range eb.handlers {
		if err := eb.client.Channel.QueueBind(
			q.Name,
			string(eventType), // routing key
			eb.exchange,
			false,
			nil,
		); err != nil {
			eb.mu.RUnlock()
			return errors.Wrapf(err, "failed to bind queue for event type %s", eventType)
		}
	}
	eb.mu.RUnlock()

	// Start consuming
	msgs, err := eb.client.Channel.Consume(
		q.Name,
		eb.consumerTag,
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return errors.Wrap(err, "failed to start consuming")
	}

	// Process messages
	eb.wg.Add(1)
	go func() {
		defer eb.wg.Done()
		for {
			select {
			case <-eb.stopChan:
				logger.Info("Stopping event bus consumer")
				return
			case msg, ok := <-msgs:
				if !ok {
					logger.Warn("Message channel closed")
					return
				}
				eb.processMessage(ctx, msg)
			}
		}
	}()

	logger.Infof("Event bus started with consumer tag: %s", eb.consumerTag)
	return nil
}

// Stop gracefully shuts down the event bus
func (eb *RabbitMQEventBus) Stop(ctx context.Context) error {
	logger := libCommons.NewLoggerFromContext(ctx)

	// Signal stop
	close(eb.stopChan)

	// Cancel consumer
	if err := eb.client.Channel.Cancel(eb.consumerTag, false); err != nil {
		logger.Errorf("Failed to cancel consumer: %v", err)
	}

	// Wait for processing to complete
	eb.wg.Wait()

	logger.Info("Event bus stopped")
	return nil
}

// processMessage handles an incoming message
func (eb *RabbitMQEventBus) processMessage(ctx context.Context, msg amqp.Delivery) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "eventbus.process_message")
	defer span.End()

	// Parse event
	var event DomainEvent
	if err := json.Unmarshal(msg.Body, &event); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal event", err)
		logger.Errorf("Failed to unmarshal event: %v", err)
		msg.Nack(false, false) // Don't requeue malformed messages
		return
	}

	// Add event metadata to span
	span.SetAttributes(
		libOpentelemetry.Attribute("event.id", event.ID.String()),
		libOpentelemetry.Attribute("event.type", string(event.Type)),
		libOpentelemetry.Attribute("event.aggregate_id", event.AggregateID.String()),
	)

	// Get handlers for this event type
	eb.mu.RLock()
	handlers := eb.handlers[event.Type]
	eb.mu.RUnlock()

	if len(handlers) == 0 {
		logger.Warnf("No handlers registered for event type: %s", event.Type)
		msg.Ack(false) // Acknowledge even if no handlers
		return
	}

	// Process event with all handlers
	var handlerErrors []error
	for _, handler := range handlers {
		if err := handler.Handle(ctx, event); err != nil {
			libOpentelemetry.HandleSpanError(&span, fmt.Sprintf("Handler failed for event %s", event.Type), err)
			logger.Errorf("Handler failed for event %s: %v", event.ID, err)
			handlerErrors = append(handlerErrors, err)
		}
	}

	// If any handler failed, nack the message for retry
	if len(handlerErrors) > 0 {
		msg.Nack(false, true) // Requeue for retry
		return
	}

	// All handlers succeeded, acknowledge the message
	msg.Ack(false)
	logger.Infof("Successfully processed event %s of type %s", event.ID, event.Type)
}