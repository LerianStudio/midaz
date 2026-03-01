// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
)

func TestServiceGetRunnablesWithOptionsConsumerDisabled(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	service := &Service{
		Logger:          logger,
		ConsumerEnabled: false,
	}

	runnables := service.GetRunnablesWithOptions(true)
	require.Len(t, runnables, 1)
	require.Equal(t, "Transaction Fiber Server", runnables[0].Name)
}

func TestServiceGetRunnablesWithOptionsConsumerDisabledIncludesGRPCWhenRequested(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	service := &Service{
		Logger:          logger,
		ConsumerEnabled: false,
	}

	runnables := service.GetRunnablesWithOptions(false)
	require.Len(t, runnables, 2)
	require.Equal(t, "Transaction Fiber Server", runnables[0].Name)
	require.Equal(t, "Transaction gRPC Server", runnables[1].Name)
}

func TestServiceGetRunnablesWithOptionsConsumerEnabledExcludeGRPC(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	service := &Service{
		Logger:          logger,
		ConsumerEnabled: true,
	}

	runnables := service.GetRunnablesWithOptions(true)
	require.Len(t, runnables, 3)
	require.Equal(t, "Transaction Fiber Server", runnables[0].Name)
	require.Equal(t, "Transaction Broker Consumer", runnables[1].Name)
	require.Equal(t, "Transaction Redis Consumer", runnables[2].Name)
}

func TestServiceGetRunnablesWithOptionsConsumerEnabled(t *testing.T) {
	t.Parallel()

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	service := &Service{
		Logger:          logger,
		ConsumerEnabled: true,
	}

	runnables := service.GetRunnablesWithOptions(false)
	require.Len(t, runnables, 4)
	require.Equal(t, "Transaction Fiber Server", runnables[0].Name)
	require.Equal(t, "Transaction Broker Consumer", runnables[1].Name)
	require.Equal(t, "Transaction Redis Consumer", runnables[2].Name)
	require.Equal(t, "Transaction gRPC Server", runnables[3].Name)
}

func TestInitConsumerWithOptionsFailsFastOnInvalidConfig(t *testing.T) {
	t.Setenv("REDIS_DB", "invalid-int")

	service, err := InitConsumerWithOptions(nil)
	require.Error(t, err)
	require.Nil(t, service)
}

func TestValidateConsumerModeConfig(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateConsumerModeConfig(&Config{DedicatedConsumerEnabled: true, ConsumerEnabled: false}))
	require.NoError(t, validateConsumerModeConfig(&Config{DedicatedConsumerEnabled: false, ConsumerEnabled: true}))

	err := validateConsumerModeConfig(&Config{DedicatedConsumerEnabled: true, ConsumerEnabled: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "CONSUMER_ENABLED must be false")

	err = validateConsumerModeConfig(&Config{DedicatedConsumerEnabled: false, ConsumerEnabled: false})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid consumer mode")
}
