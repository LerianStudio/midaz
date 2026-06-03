// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ############################################################################
// Metrics Tests (TC-MET-001 to TC-MET-002)
// ############################################################################

func TestMetrics_Endpoint(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := apiClient.GetMetricsRaw(ctx)
	require.NoError(t, err)

	// Metrics endpoint should return 200 OK
	assert.Equal(t, http.StatusOK, resp.StatusCode(), "metrics endpoint should return 200")
	assert.NotEmpty(t, resp.Body(), "metrics response should not be empty")
}
