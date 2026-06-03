// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package containers

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

const (
	RabbitUser     = "reporter-user"
	RabbitPassword = "reporter-pass"

	// Exchange names
	ExchangeGenerateReport = "reporter.generate-report.exchange"
	ExchangeDLX            = "reporter.dlx"

	// Queue names
	QueueGenerateReport = "reporter.generate-report.queue"
	QueueDLQ            = "reporter.dlq"

	// Routing keys
	RoutingKeyGenerateReport = "reporter.generate-report.key"
	RoutingKeyDLQ            = "reporter.dlq.key"

	// DLQ configuration
	dlqMessageTTLMs = 604800000 // 7 days in milliseconds
	dlqMaxLength    = 10000
)

// RabbitMQContainer wraps a RabbitMQ testcontainer with connection info.
type RabbitMQContainer struct {
	*rabbitmq.RabbitMQContainer
	AmqpURL  string
	Host     string
	AmqpPort string
	MgmtPort string
}

// StartRabbitMQ creates and starts a RabbitMQ container with pre-configured topology.
func StartRabbitMQ(ctx context.Context, networkName, image string) (*RabbitMQContainer, error) {
	if image == "" {
		image = "rabbitmq:4.0-management-alpine"
	}

	container, err := rabbitmq.Run(ctx,
		image,
		rabbitmq.WithAdminUsername(RabbitUser),
		rabbitmq.WithAdminPassword(RabbitPassword),
		testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Networks: []string{networkName},
				NetworkAliases: map[string][]string{
					networkName: {"rabbitmq", "reporter-rabbitmq"},
				},
			},
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("start rabbitmq container: %w", err)
	}

	// Get host and dynamically mapped ports
	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get rabbitmq host: %w", err)
	}

	amqpMapped, err := container.MappedPort(ctx, "5672/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get rabbitmq amqp mapped port: %w", err)
	}

	mgmtMapped, err := container.MappedPort(ctx, "15672/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("get rabbitmq mgmt mapped port: %w", err)
	}

	amqpPort := amqpMapped.Port()
	mgmtPort := mgmtMapped.Port()

	amqpURL := fmt.Sprintf("amqp://%s:%s@%s:%s/", RabbitUser, RabbitPassword, host, amqpPort)

	rc := &RabbitMQContainer{
		RabbitMQContainer: container,
		AmqpURL:           amqpURL,
		Host:              host,
		AmqpPort:          amqpPort,
		MgmtPort:          mgmtPort,
	}

	// Setup topology (exchanges, queues, bindings)
	if err := rc.setupTopology(amqpURL); err != nil {
		_ = container.Terminate(ctx)
		return nil, fmt.Errorf("setup rabbitmq topology: %w", err)
	}

	return rc, nil
}

// setupTopology creates exchanges, queues, and bindings matching definitions.json.
func (r *RabbitMQContainer) setupTopology(amqpURL string) error {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return fmt.Errorf("dial amqp: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	// Declare main exchange
	if err := ch.ExchangeDeclare(
		ExchangeGenerateReport,
		"direct",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		return fmt.Errorf("declare exchange %s: %w", ExchangeGenerateReport, err)
	}

	// Declare DLX exchange
	if err := ch.ExchangeDeclare(
		ExchangeDLX,
		"direct",
		true, false, false, false, nil,
	); err != nil {
		return fmt.Errorf("declare exchange %s: %w", ExchangeDLX, err)
	}

	// Declare main queue with DLX
	_, err = ch.QueueDeclare(
		QueueGenerateReport,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		amqp.Table{
			"x-dead-letter-exchange":    ExchangeDLX,
			"x-dead-letter-routing-key": RoutingKeyDLQ,
		},
	)
	if err != nil {
		return fmt.Errorf("declare queue %s: %w", QueueGenerateReport, err)
	}

	// Declare DLQ
	_, err = ch.QueueDeclare(
		QueueDLQ,
		true, false, false, false,
		amqp.Table{
			"x-message-ttl": int64(dlqMessageTTLMs),
			"x-max-length":  int64(dlqMaxLength),
		},
	)
	if err != nil {
		return fmt.Errorf("declare queue %s: %w", QueueDLQ, err)
	}

	// Bind main queue to exchange
	if err := ch.QueueBind(
		QueueGenerateReport,
		RoutingKeyGenerateReport,
		ExchangeGenerateReport,
		false, nil,
	); err != nil {
		return fmt.Errorf("bind queue %s: %w", QueueGenerateReport, err)
	}

	// Bind DLQ to DLX
	if err := ch.QueueBind(
		QueueDLQ,
		RoutingKeyDLQ,
		ExchangeDLX,
		false, nil,
	); err != nil {
		return fmt.Errorf("bind queue %s: %w", QueueDLQ, err)
	}

	return nil
}

// Restart stops and starts the RabbitMQ container, refreshing connection info.
func (r *RabbitMQContainer) Restart(ctx context.Context, delay time.Duration) error {
	if err := r.Stop(ctx, nil); err != nil {
		return fmt.Errorf("stop rabbitmq: %w", err)
	}

	if delay > 0 {
		time.Sleep(delay)
	}

	if err := r.Start(ctx); err != nil {
		return fmt.Errorf("start rabbitmq: %w", err)
	}

	// Host and mapped ports may change after restart
	host, err := r.RabbitMQContainer.Host(ctx)
	if err != nil {
		return fmt.Errorf("refresh rabbitmq host: %w", err)
	}

	amqpMapped, err := r.MappedPort(ctx, "5672/tcp")
	if err != nil {
		return fmt.Errorf("refresh rabbitmq amqp mapped port: %w", err)
	}

	mgmtMapped, err := r.MappedPort(ctx, "15672/tcp")
	if err != nil {
		return fmt.Errorf("refresh rabbitmq mgmt mapped port: %w", err)
	}

	r.Host = host
	r.AmqpPort = amqpMapped.Port()
	r.MgmtPort = mgmtMapped.Port()
	r.AmqpURL = fmt.Sprintf("amqp://%s:%s@%s:%s/", RabbitUser, RabbitPassword, host, r.AmqpPort)

	// Re-setup topology after restart.
	// RabbitMQ may need a few seconds to accept AMQP connections after container start.
	var topologyErr error
	for i := 0; i < 10; i++ {
		topologyErr = r.setupTopology(r.AmqpURL)
		if topologyErr == nil {
			return nil
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("re-setup topology after 10 retries: %w", topologyErr)
}

// PurgeQueues removes all messages from all queues.
func (r *RabbitMQContainer) PurgeQueues() error {
	conn, err := amqp.Dial(r.AmqpURL)
	if err != nil {
		return fmt.Errorf("dial amqp: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer ch.Close()

	if _, err := ch.QueuePurge(QueueGenerateReport, false); err != nil {
		return fmt.Errorf("purge queue %s: %w", QueueGenerateReport, err)
	}

	if _, err := ch.QueuePurge(QueueDLQ, false); err != nil {
		return fmt.Errorf("purge queue %s: %w", QueueDLQ, err)
	}

	return nil
}
