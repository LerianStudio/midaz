// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
)

type countingCloser struct {
	closed int
	err    error
}

func (c *countingCloser) Close() error {
	c.closed++
	return c.err
}

func TestServiceCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	authorizerCloser := &countingCloser{}
	producerCloser := &countingCloser{}

	logger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	service := &Service{
		Logger:           logger,
		authorizerCloser: authorizerCloser,
		brokerProducer:   producerCloser,
	}

	require.NoError(t, service.Close())
	require.NoError(t, service.Close())
	require.Equal(t, 1, authorizerCloser.closed)
	require.Equal(t, 1, producerCloser.closed)
}

func TestConsumerServiceCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	authorizerCloser := &countingCloser{}
	producerCloser := &countingCloser{}

	csLogger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	service := &ConsumerService{
		Logger:           csLogger,
		authorizerCloser: authorizerCloser,
		brokerProducer:   producerCloser,
	}

	require.NoError(t, service.Close())
	require.NoError(t, service.Close())
	require.Equal(t, 1, authorizerCloser.closed)
	require.Equal(t, 1, producerCloser.closed)
}

func TestCloseAggregatesErrors(t *testing.T) {
	t.Parallel()

	authorizerErr := errors.New("authorizer close failed") //nolint:err113
	producerErr := errors.New("producer close failed")     //nolint:err113

	aggLogger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	service := &Service{
		Logger:           aggLogger,
		authorizerCloser: &countingCloser{err: authorizerErr},
		brokerProducer:   &countingCloser{err: producerErr},
	}

	err = service.Close()
	require.Error(t, err)
	require.ErrorIs(t, err, authorizerErr)
	require.ErrorIs(t, err, producerErr)
}

func TestConsumerServiceCloseAggregatesErrors(t *testing.T) {
	t.Parallel()

	authorizerErr := errors.New("consumer authorizer close failed") //nolint:err113
	producerErr := errors.New("consumer producer close failed")     //nolint:err113

	csLogger, err := libZap.InitializeLoggerWithError()
	require.NoError(t, err)

	service := &ConsumerService{
		Logger:           csLogger,
		authorizerCloser: &countingCloser{err: authorizerErr},
		brokerProducer:   &countingCloser{err: producerErr},
	}

	err = service.Close()
	require.Error(t, err)
	require.ErrorIs(t, err, authorizerErr)
	require.ErrorIs(t, err, producerErr)
}
