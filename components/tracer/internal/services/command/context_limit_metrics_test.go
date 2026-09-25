// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	libLog "github.com/LerianStudio/lib-observability/v4/log"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/observability"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestRecordContextLimitEligibilityFailure(t *testing.T) {
	registry := prometheus.NewRegistry()
	factory, shutdown, err := observability.NewPrometheusBackedFactory(registry, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown() })

	recordContextLimitEligibilityFailure(context.Background(), factory, &libLog.NopLogger{},
		fmt.Errorf("load active limits: %w", constant.ErrContextLimitsUnavailable))
	recordContextLimitEligibilityFailure(context.Background(), factory, &libLog.NopLogger{},
		constant.ErrInvalidRequestBody)

	families, err := registry.Gather()
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() == metricContextLimitEligibilityFailures.Name {
			require.Len(t, family.GetMetric(), 1)
			require.Equal(t, float64(1), family.GetMetric()[0].GetCounter().GetValue())

			return
		}
	}

	t.Fatalf("metric %s was not emitted", metricContextLimitEligibilityFailures.Name)
}
