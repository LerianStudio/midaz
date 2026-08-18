// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	testutil_integration "github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil_integration"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func TestIntegration_ReservationOutcomeV2_BacklogMetricExistsWhenLegacyReaperDisabled(t *testing.T) {
	cleanup, err := testutil_integration.RestartServerWithConfig(map[string]string{
		"RESERVATION_REAPER_ENABLED":          "false",
		"RESERVATION_REAPER_INTERVAL_SECONDS": "1",
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, cleanup()) }()

	db := testutil.SetupIntegrationDB(t)
	limitID := resSeedLimit(t, db, 9961, "v2-metrics-wiring", 10000)
	t.Cleanup(func() { resCleanupLimit(t, db, limitID) })

	transactionID := testutil.MustDeterministicUUID(9962)
	service := resWireService(t, db, resStubResolver{specs: []query.ReservationSpec{
		resSpec(limitID, "v2:9961", "2026-08", 10, 10000),
	}}, &resCountingAudit{})
	_, err = service.Reserve(t.Context(), transactionID, &model.CheckLimitsInput{}, false, model.DeliveryModeLedgerOutcomeV2)
	require.NoError(t, err)

	var lastMetrics string
	found := assert.Eventually(t, func() bool {
		response, requestErr := http.Get(testutil.GetBaseURL() + "/metrics") //nolint:gosec,noctx
		if requestErr != nil {
			return false
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return false
		}

		body, readErr := io.ReadAll(response.Body)
		lastMetrics = string(body)
		return readErr == nil && strings.Contains(lastMetrics, "tracer_reservation_v2_outstanding 1")
	}, 5*time.Second, 100*time.Millisecond, "observer-only production wiring must publish the V2 backlog")
	if !found {
		t.Logf("last /metrics payload:\n%s", lastMetrics)
	}
	require.True(t, found)
}
