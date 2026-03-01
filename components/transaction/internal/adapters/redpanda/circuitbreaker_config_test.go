// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redpanda

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProducerCircuitBreakerConfig(t *testing.T) {
	t.Parallel()

	cfg := CircuitBreakerConfig{
		ConsecutiveFailures: 3,
		FailureRatio:        0.4,
		Interval:            30 * time.Second,
		MaxRequests:         2,
		MinRequests:         5,
		Timeout:             10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	}

	resolved := ProducerCircuitBreakerConfig(cfg)

	assert.Equal(t, cfg.ConsecutiveFailures, resolved.ConsecutiveFailures)
	assert.InEpsilon(t, cfg.FailureRatio, resolved.FailureRatio, 1e-9)
	assert.Equal(t, cfg.Interval, resolved.Interval)
	assert.Equal(t, cfg.MaxRequests, resolved.MaxRequests)
	assert.Equal(t, cfg.MinRequests, resolved.MinRequests)
	assert.Equal(t, cfg.Timeout, resolved.Timeout)
}
