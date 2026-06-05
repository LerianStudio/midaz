// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"

	rabbitmqadapter "github.com/LerianStudio/midaz/v4/components/reporter-worker/internal/adapters/rabbitmq"
	reportData "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/report"

	libRabbitMQ "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	tmmongo "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/mongo"
	clog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
)

// initConsumerRoutes creates consumer routes for single-tenant mode.
// Multi-tenant mode uses initMultiTenantConsumer + NewMultiQueueConsumerMultiTenant instead.
func initConsumerRoutes(
	rabbitMQConnection *libRabbitMQ.RabbitMQConnection,
	numWorkers int,
	logger clog.Logger,
	telemetry *libOtel.Telemetry,
	tenantMongoManager *tmmongo.Manager,
	reportMongoDBRepository *reportData.ReportMongoDBRepository,
) (*rabbitmqadapter.ConsumerRoutes, error) {
	routes, err := rabbitmqadapter.NewConsumerRoutes(rabbitMQConnection, numWorkers, logger, telemetry, tenantMongoManager, reportMongoDBRepository)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize rabbitmq consumer: %w", err)
	}

	return routes, nil
}

// closeRabbitMQ returns a cleanup function that safely closes
// the RabbitMQ channel and connection.
func closeRabbitMQ(conn *libRabbitMQ.RabbitMQConnection, logger clog.Logger) func() {
	return func() {
		logger.Log(context.Background(), clog.LevelInfo, "Cleanup: closing RabbitMQ connection")

		if conn.Channel != nil && !conn.Channel.IsClosed() {
			if closeErr := conn.Channel.Close(); closeErr != nil {
				logger.Log(context.Background(), clog.LevelError, "Cleanup: failed to close RabbitMQ channel", clog.Err(closeErr))
			}
		}

		if conn.Connection != nil && !conn.Connection.IsClosed() {
			if closeErr := conn.Connection.Close(); closeErr != nil {
				logger.Log(context.Background(), clog.LevelError, "Cleanup: failed to close RabbitMQ connection", clog.Err(closeErr))
			}
		}
	}
}
