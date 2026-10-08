//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerRabbitMQ "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// TestIntegrationEngineWriteBehindPublishesBalanceEventsOncePerOperation drives a
// pending create and its commit or cancel through the write-behind consumer. The
// transition's completion re-completes the pending origin as its predecessor, and
// a broker can redeliver either message; neither may repeat a balance.changed that
// the completion which wrote the operation already published.
func TestIntegrationEngineWriteBehindPublishesBalanceEventsOncePerOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	producer, err := ledgerRabbitMQ.NewSingleTenantEngineWriteBehindProducer(
		infra.rabbitConn, infra.exchange, infra.routingKey, time.Second,
	)
	require.NoError(t, err)
	infra.command.TransactionWriteBehindDispatcher = engineWriteBehindDispatcher{publisher: producer}
	recorder := pkgStreaming.NewMockEmitter()
	infra.command.Streaming = recorder
	app := infra.newHTTPApp("")
	dispatcher := requireRabbitTransactionDispatcher(t, infra.command, rabbitEngineSingleTenant, false)

	for _, testCase := range []struct {
		version string
		action  string
		status  string
	}{
		{version: "v1", action: "commit", status: constant.APPROVED},
		{version: "v2", action: "cancel", status: constant.CANCELED},
	} {
		t.Run(testCase.version+" "+testCase.action+" after the pending projection", func(t *testing.T) {
			ctx := context.Background()
			aliases := infra.seedTransfer(t, "events-after-"+testCase.action+"-"+strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
			created := infra.postPendingCreate(t, app, testCase.version, aliases)
			require.Equalf(t, http.StatusCreated, created.status, "async pending create must return 201: %s", created.body)
			transactionID := created.transactionID(t)

			createDelivery := infra.nextWriteBehindDelivery(t)
			require.NoError(t, dispatcher.handle(ctx, createDelivery.Body))
			require.NoError(t, createDelivery.Ack(false))
			infra.requireBalanceChangedOncePerOperation(t, recorder, transactionID)

			transition := infra.postTransition(t, app, testCase.version, transactionID, testCase.action)
			require.Equalf(t, http.StatusCreated, transition.status, "%s must succeed: %s", testCase.action, transition.body)

			transitionDelivery := infra.nextWriteBehindDelivery(t)
			require.NoError(t, dispatcher.handle(ctx, transitionDelivery.Body))
			require.NoError(t, transitionDelivery.Ack(false))
			infra.requireBalanceChangedOncePerOperation(t, recorder, transactionID)

			require.NoError(t, dispatcher.handle(ctx, transitionDelivery.Body), "a redelivered transition converges")
			infra.requireBalanceChangedOncePerOperation(t, recorder, transactionID)
		})
	}

	t.Run("v2 commit completed before its pending origin", func(t *testing.T) {
		ctx := context.Background()
		aliases := infra.seedTransfer(t, "events-before-commit-"+strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
		created := infra.postPendingCreate(t, app, "v2", aliases)
		require.Equalf(t, http.StatusCreated, created.status, "async pending create must return 201: %s", created.body)
		transactionID := created.transactionID(t)

		transition := infra.postTransition(t, app, "v2", transactionID, "commit")
		require.Equalf(t, http.StatusCreated, transition.status, "commit before projection must succeed: %s", transition.body)

		createDelivery := infra.nextWriteBehindDelivery(t)
		transitionDelivery := infra.nextWriteBehindDelivery(t)

		require.NoError(t, dispatcher.handle(ctx, transitionDelivery.Body), "the commit projects its pending origin first")
		require.NoError(t, transitionDelivery.Ack(false))
		require.NoError(t, dispatcher.handle(ctx, createDelivery.Body), "the origin's own message replays as noop")
		require.NoError(t, createDelivery.Ack(false))

		var projected string
		require.NoError(t, infra.db.QueryRow(`SELECT status FROM transaction WHERE id = $1`, transactionID).Scan(&projected))
		assert.Equal(t, constant.APPROVED, projected)
		infra.requireBalanceChangedOncePerOperation(t, recorder, transactionID)
	})
}

func (infra *engineWriteBehindHTTPIntegration) nextWriteBehindDelivery(t *testing.T) amqp.Delivery {
	t.Helper()

	delivery, queued, err := infra.rabbit.Channel.Get(infra.queue, false)
	require.NoError(t, err)
	require.True(t, queued, "the write-behind evidence must be queued")

	return delivery
}

// requireBalanceChangedOncePerOperation waits until every balance-affecting
// operation projected for the transaction has its balance.changed event, then
// holds a short window so a late duplicate from an asynchronous fan-out is seen.
func (infra *engineWriteBehindHTTPIntegration) requireBalanceChangedOncePerOperation(t *testing.T, recorder *pkgStreaming.MockEmitter, transactionID uuid.UUID) {
	t.Helper()

	rows, err := infra.db.Query(`SELECT id FROM operation WHERE transaction_id = $1 AND balance_affected`, transactionID)
	require.NoError(t, err)

	expected := map[string]int{}

	for rows.Next() {
		var operationID string
		require.NoError(t, rows.Scan(&operationID))

		expected[transactionID.String()+":"+operationID] = 1
	}

	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.NotEmpty(t, expected, "the transaction must have projected balance-affecting operations")

	observed := func() map[string]int {
		counts := map[string]int{}

		for _, event := range recorder.Events() {
			if event.DefinitionKey == "balance.changed" && strings.HasPrefix(event.Subject, transactionID.String()+":") {
				counts[event.Subject]++
			}
		}

		return counts
	}

	require.Eventually(t, func() bool { return len(observed()) == len(expected) },
		5*time.Second, 10*time.Millisecond, "every balance-affecting operation publishes balance.changed")

	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, expected, observed(), "each operation publishes balance.changed exactly once")
}
