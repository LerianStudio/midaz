// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func TestNewM2MMetrics_CreatesAllInstruments(t *testing.T) {
	t.Parallel()

	provider := sdkmetric.NewMeterProvider()
	defer provider.Shutdown(t.Context())

	meter := provider.Meter("test")

	metrics, err := NewM2MMetrics(meter)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	assert.NotNil(t, metrics.L1CacheHits, "L1CacheHits counter must be created")
	assert.NotNil(t, metrics.L2CacheHits, "L2CacheHits counter must be created")
	assert.NotNil(t, metrics.CacheMisses, "CacheMisses counter must be created")
	assert.NotNil(t, metrics.FetchErrors, "FetchErrors counter must be created")
	assert.NotNil(t, metrics.FetchDuration, "FetchDuration histogram must be created")
	assert.NotNil(t, metrics.Invalidations, "Invalidations counter must be created")
}

func TestNoopM2MMetrics_ReturnsNonNilInstruments(t *testing.T) {
	t.Parallel()

	metrics := NoopM2MMetrics()
	require.NotNil(t, metrics)

	assert.NotNil(t, metrics.L1CacheHits, "L1CacheHits must not be nil")
	assert.NotNil(t, metrics.L2CacheHits, "L2CacheHits must not be nil")
	assert.NotNil(t, metrics.CacheMisses, "CacheMisses must not be nil")
	assert.NotNil(t, metrics.FetchErrors, "FetchErrors must not be nil")
	assert.NotNil(t, metrics.FetchDuration, "FetchDuration must not be nil")
	assert.NotNil(t, metrics.Invalidations, "Invalidations must not be nil")
}
