// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"io"

	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
)

type closeResourcesParams struct {
	circuitBreaker     *CircuitBreakerManager
	multiQueueConsumer *MultiQueueConsumer
	authorizerCloser   io.Closer
	brokerProducer     io.Closer
	redisConnection    *libRedis.RedisConnection
	postgresConnection *libPostgres.PostgresConnection
	mongoConnection    *libMongo.MongoConnection
	telemetry          *libOpentelemetry.Telemetry
}

func closeSharedResources(params closeResourcesParams) error { //nolint:gocyclo,cyclop // shutdown sequence; each resource requires its own nil guard and error collection
	var closeErrs []error

	if params.circuitBreaker != nil {
		params.circuitBreaker.Stop()
	}

	if params.multiQueueConsumer != nil && params.multiQueueConsumer.consumerRoutes != nil {
		params.multiQueueConsumer.consumerRoutes.Stop()
	}

	if params.authorizerCloser != nil {
		if err := params.authorizerCloser.Close(); err != nil {
			closeErrs = append(closeErrs, err)
		}
	}

	if params.brokerProducer != nil {
		if err := params.brokerProducer.Close(); err != nil {
			closeErrs = append(closeErrs, err)
		}
	}

	if params.redisConnection != nil {
		if err := params.redisConnection.Close(); err != nil {
			closeErrs = append(closeErrs, err)
		}
	}

	if params.postgresConnection != nil && params.postgresConnection.ConnectionDB != nil {
		if err := (*params.postgresConnection.ConnectionDB).Close(); err != nil {
			closeErrs = append(closeErrs, err)
		}
	}

	if params.mongoConnection != nil && params.mongoConnection.DB != nil {
		if err := params.mongoConnection.DB.Disconnect(context.Background()); err != nil {
			closeErrs = append(closeErrs, err)
		}
	}

	if params.telemetry != nil {
		params.telemetry.ShutdownTelemetry()
	}

	return errors.Join(closeErrs...)
}
