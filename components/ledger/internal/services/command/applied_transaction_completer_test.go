// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestAcknowledgeEngineRecoveryEmitsDeferredMetricWithoutFailing(t *testing.T) {
	reader, factory := newReaderFactory(t)
	acknowledger := &recordingEngineRecoveryAcknowledger{err: errors.New("redis unavailable")}
	useCase := &UseCase{EngineRecoveryAcknowledger: acknowledger, MetricsFactory: factory}
	record := &TransactionCompletionRecord{TransactionID: uuid.New(), ExecutionID: uuid.New()}

	useCase.acknowledgeEngineRecovery(
		context.Background(),
		libLog.NewNop(),
		record,
		TransactionCompletionResult{Outcome: TransactionPersistenceOutcome{TransactionStatus: "APPROVED"}},
	)

	require.Len(t, acknowledger.records, 1)

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))

	for _, scope := range data.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name != engineRecoveryAckDeferredMetricName {
				continue
			}

			counter, ok := item.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			require.Len(t, counter.DataPoints, 1)
			require.Equal(t, int64(1), counter.DataPoints[0].Value)
			require.Zero(t, counter.DataPoints[0].Attributes.Len())
			return
		}
	}

	t.Fatal("deferred engine recovery acknowledgment metric was not emitted")
}
