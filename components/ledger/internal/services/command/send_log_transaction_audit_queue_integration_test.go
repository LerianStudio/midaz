//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	libRabbitmq "github.com/LerianStudio/lib-commons/v7/commons/rabbitmq"
	libZap "github.com/LerianStudio/lib-observability/v4/zap"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	rmqtestutil "github.com/LerianStudio/midaz/v4/tests/utils/rabbitmq"
)

const (
	auditIntegrationRunID       = "g38-audit"
	auditIntegrationWaitTimeout = 10 * time.Second
)

// auditIntegrationInfra is a real producer bound to a RabbitMQ virtual host owned by one test.
type auditIntegrationInfra struct {
	broker   *rmqtestutil.ContainerResult
	conn     *libRabbitmq.RabbitMQConnection
	producer *rabbitmq.ProducerRabbitMQRepository
	uc       *UseCase
}

func TestIntegration_AuditLog(t *testing.T) {
	t.Run("audit-default-off-env-ausente", func(t *testing.T) {
		t.Setenv("ALLOW_INSECURE_TLS", "true")
		unsetAuditLogEnabled(t)

		auditExchange := auditIntegrationName(t, "audit.exchange")
		auditKey := auditIntegrationName(t, "audit.key")
		t.Setenv("RABBITMQ_AUDIT_EXCHANGE", auditExchange)
		t.Setenv("RABBITMQ_AUDIT_KEY", auditKey)

		infra := setupAuditIntegrationInfra(t)

		sentinelExchange := auditIntegrationName(t, "sentinel.exchange")
		sentinelKey := auditIntegrationName(t, "sentinel.key")
		sentinelQueue := auditIntegrationName(t, "sentinel.queue")
		rmqtestutil.SetupExchange(t, infra.broker.Channel, sentinelExchange, "direct")
		rmqtestutil.SetupQueue(t, infra.broker.Channel, sentinelQueue, sentinelExchange, sentinelKey)

		channelBefore := infra.conn.ChannelSnapshot()
		require.NotNil(t, channelBefore, "producer must hold an open channel before the call")

		infra.uc.SendLogTransactionAuditQueue(context.Background(), auditTestOperations(), auditTestOrganizationID, auditTestLedgerID, auditTestTransactionID)

		// A synchronous round trip on the producer channel fails if the broker closed it
		// for a publish to the undeclared audit exchange.
		channelAfter := infra.conn.ChannelSnapshot()
		require.Same(t, channelBefore, channelAfter, "producer channel must not be replaced")
		_, err := channelAfter.QueueDeclarePassive(sentinelQueue, true, false, false, false, nil)
		require.NoError(t, err, "producer channel must stay usable after the call")
		assert.False(t, channelAfter.IsClosed(), "producer channel must stay open after the call")

		_, err = infra.producer.ProducerDefault(context.Background(), sentinelExchange, sentinelKey, []byte(`{"probe":true}`))
		require.NoError(t, err, "a later publish through the same producer must succeed")
		rmqtestutil.WaitForQueueCount(t, infra.broker.Channel, sentinelQueue, 1, auditIntegrationWaitTimeout)
		assert.Same(t, channelBefore, infra.conn.ChannelSnapshot(), "producer channel must not be reopened")
	})

	t.Run("audit-on-com-true-explicito", func(t *testing.T) {
		t.Setenv("ALLOW_INSECURE_TLS", "true")
		t.Setenv("AUDIT_LOG_ENABLED", "true")

		auditExchange := auditIntegrationName(t, "audit.exchange")
		auditKey := auditIntegrationName(t, "audit.key")
		auditQueue := auditIntegrationName(t, "audit.queue")
		t.Setenv("RABBITMQ_AUDIT_EXCHANGE", auditExchange)
		t.Setenv("RABBITMQ_AUDIT_KEY", auditKey)

		infra := setupAuditIntegrationInfra(t)
		rmqtestutil.SetupExchange(t, infra.broker.Channel, auditExchange, "direct")
		rmqtestutil.SetupQueue(t, infra.broker.Channel, auditQueue, auditExchange, auditKey)

		operations := auditTestOperations()

		infra.uc.SendLogTransactionAuditQueue(context.Background(), operations, auditTestOrganizationID, auditTestLedgerID, auditTestTransactionID)

		rmqtestutil.WaitForQueueCount(t, infra.broker.Channel, auditQueue, 1, auditIntegrationWaitTimeout)

		delivery, ok, err := infra.broker.Channel.Get(auditQueue, true)
		require.NoError(t, err)
		require.True(t, ok, "one audit message must be available")

		var message mmodel.Queue
		require.NoError(t, json.Unmarshal(delivery.Body, &message))

		assert.Equal(t, auditTestOrganizationID, message.OrganizationID)
		assert.Equal(t, auditTestLedgerID, message.LedgerID)
		assert.Equal(t, auditTestTransactionID, message.AuditID)
		require.Len(t, message.QueueData, len(operations))

		for i, op := range operations {
			assert.Equal(t, uuid.MustParse(op.ID), message.QueueData[i].ID)
		}

		assert.Equal(t, 0, rmqtestutil.GetQueueMessageCount(t, infra.broker.Channel, auditQueue), "exactly one audit message must be published")
	})

	t.Run("audit-on-exchange-inexistente-nao-afeta-transacao", func(t *testing.T) {
		t.Setenv("ALLOW_INSECURE_TLS", "true")
		t.Setenv("AUDIT_LOG_ENABLED", "true")

		t.Setenv("RABBITMQ_AUDIT_EXCHANGE", auditIntegrationName(t, "audit.exchange"))
		t.Setenv("RABBITMQ_AUDIT_KEY", auditIntegrationName(t, "audit.key"))

		infra := setupAuditIntegrationInfra(t)

		channelBefore := infra.conn.ChannelSnapshot()
		require.NotNil(t, channelBefore)

		// The broker closes the producer channel asynchronously with 404 NOT_FOUND when the
		// publish targets an undeclared exchange; the close notification proves the attempt.
		closed := channelBefore.NotifyClose(make(chan *amqp.Error, 1))

		done := make(chan struct{})

		go func() {
			defer close(done)

			infra.uc.SendLogTransactionAuditQueue(context.Background(), auditTestOperations(), auditTestOrganizationID, auditTestLedgerID, auditTestTransactionID)
		}()

		select {
		case <-done:
		case <-time.After(auditIntegrationWaitTimeout):
			require.FailNow(t, "audit publication to an undeclared exchange did not return within the timeout")
		}

		select {
		case closeErr := <-closed:
			require.NotNil(t, closeErr, "producer channel must be closed by the broker, not by the client")
			require.Equal(t, amqp.NotFound, closeErr.Code)
		case <-time.After(auditIntegrationWaitTimeout):
			require.FailNow(t, "broker did not close the producer channel within the timeout")
		}

		assert.True(t, channelBefore.IsClosed(), "broker closes the producer channel on a publish to an undeclared exchange")
	})
}

// setupAuditIntegrationInfra wires a UseCase to a real producer on a dedicated virtual host.
func setupAuditIntegrationInfra(t *testing.T) *auditIntegrationInfra {
	t.Helper()

	broker := rmqtestutil.SetupReusableContainer(t)

	logger, err := libZap.New(libZap.Config{Environment: libZap.EnvironmentDevelopment, OTelLibraryName: "midaz-tests"})
	require.NoError(t, err)

	conn := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource:   broker.URI,
		HealthCheckURL:           "http://" + broker.Host + ":" + broker.MgmtPort,
		AllowInsecureHealthCheck: true,
		Host:                     broker.Host,
		Port:                     broker.AMQPPort,
		User:                     rmqtestutil.DefaultUser,
		Pass:                     rmqtestutil.DefaultPassword,
		Logger:                   logger,
	}

	producer, err := rabbitmq.NewProducerRabbitMQ(conn)
	require.NoError(t, err, "failed to create producer")

	t.Cleanup(func() {
		_ = conn.Close()
	})

	return &auditIntegrationInfra{
		broker:   broker,
		conn:     conn,
		producer: producer,
		uc:       &UseCase{RabbitMQRepo: producer},
	}
}

// auditIntegrationName builds a broker object name unique to the run and the calling test.
func auditIntegrationName(t *testing.T, base string) string {
	t.Helper()

	return base + "." + auditIntegrationRunID + "." + strings.ReplaceAll(t.Name(), "/", ".")
}
