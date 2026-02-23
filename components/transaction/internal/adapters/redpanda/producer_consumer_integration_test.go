//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	redpandatestutil "github.com/LerianStudio/midaz/v3/tests/utils/redpanda"
	"github.com/stretchr/testify/require"
)

func TestConsumerRoutes_RetryFlow_Integration(t *testing.T) {
	if os.Getenv("RUN_REDPANDA_IT") != "1" {
		t.Skip("set RUN_REDPANDA_IT=1 to run Redpanda integration tests")
	}

	container := redpandatestutil.SetupContainer(t)

	topic := "it.balance.operations." + libCommons.GenerateUUIDv7().String()
	group := "it-consumer-group-" + libCommons.GenerateUUIDv7().String()

	redpandatestutil.SetupTopics(t, container, topic, topic+retryTopicSuffix, topic+dltTopicSuffix)

	logger := libZap.InitializeLogger()
	telemetry := &libOpentelemetry.Telemetry{}

	routes := NewConsumerRoutesWithSecurity(
		container.Brokers,
		group,
		1,
		0,
		logger,
		telemetry,
		ClientSecurityConfig{},
		2,
	)
	t.Cleanup(routes.Stop)

	producer, err := NewProducerRedpanda(container.Brokers, 0, 0, true)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, producer.Close())
	})

	processed := make(chan struct{}, 1)
	var attempts atomic.Int32

	routes.Register(topic, func(_ context.Context, body []byte) error {
		if string(body) != "hello" {
			return errors.New("unexpected payload")
		}

		if attempts.Add(1) == 1 {
			return errors.New("force retry")
		}

		processed <- struct{}{}

		return nil
	})

	require.NoError(t, routes.RunConsumers())

	_, err = producer.ProducerDefaultWithContext(context.Background(), topic, "key-1", []byte("hello"))
	require.NoError(t, err)

	select {
	case <-processed:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for retried message to be processed")
	}

	require.GreaterOrEqual(t, attempts.Load(), int32(2))
}
