// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"testing"
	"time"

	pkg "github.com/LerianStudio/midaz/v3/pkg/reporter"
	pkgConstant "github.com/LerianStudio/midaz/v3/pkg/reporter/constant"
	pkgRabbitmq "github.com/LerianStudio/midaz/v3/pkg/reporter/rabbitmq"

	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConsumerRetryManager(t *testing.T) {
	t.Parallel()

	classifier := pkgRabbitmq.NewDefaultErrorClassifier()
	backoff := &pkg.BackoffCalculator{
		InitialDelay: pkgConstant.RetryInitialBackoff,
		MaxDelay:     pkgConstant.RetryMaxBackoff,
		Factor:       2.0,
	}
	logger := log.NewNop()

	telemetry := libOtel.Telemetry{}

	rm := NewConsumerRetryManager(classifier, backoff, nil, nil, logger, telemetry)

	require.NotNil(t, rm)
	assert.Equal(t, classifier, rm.classifier)
	assert.Equal(t, backoff, rm.backoff)
	assert.Nil(t, rm.conn)
	assert.Nil(t, rm.rabbitMQManager)
	assert.Equal(t, logger, rm.logger)
	assert.Equal(t, telemetry, rm.telemetry)
	assert.Equal(t, pkgConstant.MaxMessageRetries, rm.maxRetries)
	assert.NotNil(t, rm.sleepFunc, "sleepFunc must default to a non-nil function")
}

func TestNewConsumerRetryManager_MaxRetriesFromConstant(t *testing.T) {
	t.Parallel()

	rm := NewConsumerRetryManager(
		pkgRabbitmq.NewDefaultErrorClassifier(),
		&pkg.BackoffCalculator{},
		nil,
		nil,
		log.NewNop(),
		libOtel.Telemetry{},
	)

	assert.Equal(t, pkgConstant.MaxMessageRetries, rm.maxRetries,
		"maxRetries must be set from pkgConstant.MaxMessageRetries")
}

func TestNewConsumerRetryManager_SleepFuncDefaultsToTimeSleep(t *testing.T) {
	t.Parallel()

	rm := NewConsumerRetryManager(
		pkgRabbitmq.NewDefaultErrorClassifier(),
		&pkg.BackoffCalculator{},
		nil,
		nil,
		log.NewNop(),
		libOtel.Telemetry{},
	)

	// Verify the sleepFunc is set (we can't compare function pointers in Go,
	// but we can verify it's not nil and call it with zero duration).
	require.NotNil(t, rm.sleepFunc)

	// Calling with zero duration should return immediately without panic.
	assert.NotPanics(t, func() {
		rm.sleepFunc(0 * time.Millisecond)
	})
}
