// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Channel abstracts an AMQP channel for publishing messages.
// The manager producer uses this interface directly via type alias.
// The worker consumer still uses a legacy interface (RabbitMQConnectionChannel)
// with Publish() instead of PublishWithContext() — migration tracked as tech debt.
type Channel interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	Close() error
}

// TenantChannelManager provides per-tenant channel isolation via RabbitMQ vhosts.
// The manager producer uses this interface directly via type alias.
// The worker consumer still uses a legacy interface (RabbitMQManagerConsumerInterface)
// with GetConnection() — migration tracked as tech debt.
type TenantChannelManager interface {
	GetChannel(ctx context.Context, tenantID string) (Channel, error)
}
