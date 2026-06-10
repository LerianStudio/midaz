// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"testing"
	"time"

	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	pkgRabbitmq "github.com/LerianStudio/midaz/v4/pkg/reporter/rabbitmq"

	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConsumerRetryManager(t *testing.T) {
	t.Parallel()

	classifier := pkgRabbitmq.NewDefaultErrorClassifier()
	backoff := func(int) time.Duration { return 0 }
	logger := log.NewNop()

	telemetry := libOtel.Telemetry{}

	rm := NewConsumerRetryManager(classifier, backoff, nil, nil, logger, telemetry)

	require.NotNil(t, rm)
	assert.Equal(t, classifier, rm.classifier)
	require.NotNil(t, rm.backoff, "backoff func must be set")
	assert.Nil(t, rm.conn)
	assert.Nil(t, rm.rabbitMQManager)
	assert.Equal(t, logger, rm.logger)
	assert.Equal(t, telemetry, rm.telemetry)
	assert.Equal(t, pkgConstant.MaxMessageRetries, rm.maxRetries)
}

func TestNewConsumerRetryManager_MaxRetriesFromConstant(t *testing.T) {
	t.Parallel()

	rm := NewConsumerRetryManager(
		pkgRabbitmq.NewDefaultErrorClassifier(),
		func(int) time.Duration { return 0 },
		nil,
		nil,
		log.NewNop(),
		libOtel.Telemetry{},
	)

	assert.Equal(t, pkgConstant.MaxMessageRetries, rm.maxRetries,
		"maxRetries must be set from pkgConstant.MaxMessageRetries")
}
