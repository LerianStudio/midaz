// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/metrics"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"
)

// newReaderFactory builds a real MetricsFactory backed by a manual reader so the
// test can inspect emitted domain-operation points after exercising a use case.
func newReaderFactory(t *testing.T) (*sdkmetric.ManualReader, *metrics.MetricsFactory) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(mp.Meter("domain-metrics-test"), nil)
	require.NoError(t, err)

	return reader, factory
}

// collectDomainCounters returns a map of "component/operation/result" -> value
// for the domain_operations_total counter.
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

// TestRecordDomainOperation_LedgerCommand verifies the ledger command layer emits
// domain_operations_total through the real factory with the correct
// component/operation/result labels for both a success and a technical-error path.
func TestRecordDomainOperation_LedgerCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reader, factory := newReaderFactory(t)

	mockRepo := segment.NewMockRepository(ctrl)

	uc := &UseCase{
		SegmentRepo:    mockRepo,
		MetricsFactory: factory,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Success path.
	mockRepo.EXPECT().
		ExistsByName(gomock.Any(), gomock.Any(), gomock.Any(), "Metric Segment").
		Return(false, nil)
	mockRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(&mmodel.Segment{ID: uuid.New().String(), Name: "Metric Segment"}, nil)

	_, err := uc.CreateSegment(ctx, orgID, ledgerID, &mmodel.CreateSegmentInput{
		Name:   "Metric Segment",
		Status: mmodel.Status{Code: "ACTIVE"},
	})
	require.NoError(t, err)

	// Technical-error path: ExistsByName returns a plain (non-business) error.
	mockRepo.EXPECT().
		ExistsByName(gomock.Any(), gomock.Any(), gomock.Any(), "Failing Segment").
		Return(false, errors.New("connection refused"))

	_, err = uc.CreateSegment(ctx, orgID, ledgerID, &mmodel.CreateSegmentInput{
		Name:   "Failing Segment",
		Status: mmodel.Status{Code: "ACTIVE"},
	})
	require.Error(t, err)
	require.False(t, pkg.IsBusinessError(err), "guard: error must classify as technical")

	totals := collectDomainCounters(t, reader)

	require.Equal(t, int64(1), totals["ledger/create_segment/success"],
		"one successful create_segment must be counted")
	require.Equal(t, int64(1), totals["ledger/create_segment/technical_error"],
		"one technical-error create_segment must be counted")
}

// TestRecordDomainOperation_LedgerCommand_BusinessError verifies a business error
// maps to the business_error result label (via pkg.IsBusinessError).
func TestRecordDomainOperation_LedgerCommand_BusinessError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reader, factory := newReaderFactory(t)

	mockRepo := segment.NewMockRepository(ctrl)

	uc := &UseCase{
		SegmentRepo:    mockRepo,
		MetricsFactory: factory,
	}

	ctx := context.Background()

	// ExistsByName returns a typed business error: create_segment must classify it
	// as business_error, not technical_error.
	bizErr := pkg.ValidateBusinessError(constant.ErrDuplicateSegmentName, constant.EntitySegment, "Dup")
	mockRepo.EXPECT().
		ExistsByName(gomock.Any(), gomock.Any(), gomock.Any(), "Dup").
		Return(false, bizErr)

	_, err := uc.CreateSegment(ctx, uuid.New(), uuid.New(), &mmodel.CreateSegmentInput{
		Name:   "Dup",
		Status: mmodel.Status{Code: "ACTIVE"},
	})
	require.Error(t, err)
	require.True(t, pkg.IsBusinessError(err), "guard: error must classify as business")

	totals := collectDomainCounters(t, reader)
	require.Equal(t, int64(1), totals["ledger/create_segment/business_error"],
		"one business-error create_segment must be counted")
}
