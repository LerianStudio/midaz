// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/metrics"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
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

	factory, err := metrics.NewMetricsFactory(mp.Meter("crm-domain-metrics-test"), nil)
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

// TestRecordDomainOperation_CRM verifies the CRM use-case layer emits
// domain_operations_total through the real factory with the crm component label
// for both a success and a technical-error path.
func TestRecordDomainOperation_CRM(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reader, factory := newReaderFactory(t)

	mockRepo := holder.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderRepo:     mockRepo,
		MetricsFactory: factory,
	}

	ctx := context.Background()
	const orgID = "0194ffee-e14f-70f5-b400-04b7b7434131"

	name := "Metric Holder"
	document := "90217469051"

	// Success path.
	mockRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Holder{Name: &name, Document: &document}, nil)

	_, err := uc.CreateHolder(ctx, orgID, &mmodel.CreateHolderInput{Name: name, Document: document})
	require.NoError(t, err)

	// Technical-error path.
	mockRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("connection refused"))

	_, err = uc.CreateHolder(ctx, orgID, &mmodel.CreateHolderInput{Name: name, Document: document})
	require.Error(t, err)

	totals := collectDomainCounters(t, reader)

	require.Equal(t, int64(1), totals["crm/create_holder/success"],
		"one successful create_holder must be counted")
	require.Equal(t, int64(1), totals["crm/create_holder/technical_error"],
		"one technical-error create_holder must be counted")
}
