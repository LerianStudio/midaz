// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/metrics"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"
)

func newReaderFactory(t *testing.T) (*sdkmetric.ManualReader, *metrics.MetricsFactory) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(mp.Meter("fees-domain-metrics-test"), nil)
	require.NoError(t, err)

	return reader, factory
}

func collectDomainCounters(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	totals := make(map[string]int64)

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "domain_operations_total" {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				comp, _ := dp.Attributes.Value("component")
				op, _ := dp.Attributes.Value("operation")
				res, _ := dp.Attributes.Value("result")
				key := comp.AsString() + "/" + op.AsString() + "/" + res.AsString()
				totals[key] = dp.Value
			}
		}
	}

	return totals
}

// TestRecordDomainOperation_Fees verifies the fee use-case layer emits
// domain_operations_total through the real factory with the fees component label
// for both a success and a technical-error path.
func TestRecordDomainOperation_Fees(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reader, factory := newReaderFactory(t)

	mockPackRepo := pack.NewMockRepository(ctrl)

	uc := &UseCase{
		packageRepo:    mockPackRepo,
		MetricsFactory: factory,
	}

	ctx := context.Background()
	packID := uuid.New()
	orgID := uuid.New()

	// Success path.
	mockPackRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	require.NoError(t, uc.DeletePackageByID(ctx, packID, orgID))

	// Technical-error path.
	mockPackRepo.EXPECT().
		SoftDelete(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("connection refused"))

	require.Error(t, uc.DeletePackageByID(ctx, packID, orgID))

	totals := collectDomainCounters(t, reader)

	require.Equal(t, int64(1), totals["fees/delete_package/success"],
		"one successful delete_package must be counted")
	require.Equal(t, int64(1), totals["fees/delete_package/technical_error"],
		"one technical-error delete_package must be counted")
}
