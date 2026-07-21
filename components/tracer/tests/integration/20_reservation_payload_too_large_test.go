// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// TestReservation_PayloadTooLarge mirrors TestValidation_1_1_9_PayloadTooLarge
// for the reservation endpoint: a POST /v1/reservations body larger than the
// 100KB limit must be rejected with 413 Payload Too Large and code 0143. The
// size guard runs before JSON parsing, so the oversized body need not be valid
// JSON. This is the coverage that was missing when the guard incorrectly mapped
// to 400 instead of 413.
func TestReservation_PayloadTooLarge(t *testing.T) {
	baseURL := testutil.GetBaseURL()
	apiKey := testutil.GetAPIKey()

	// 110KB body to comfortably exceed the 100KB (100*1024) limit.
	largeBody := bytes.Repeat([]byte("x"), 110*1024)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/reservations", bytes.NewReader(largeBody))
	require.NoError(t, err)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := testutil.HTTPClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode,
		"Reservation payload >100KB should return 413 Payload Too Large, got body: %s", string(respBody))

	errorResp := testutil.ParseErrorResponse(t, respBody)
	assert.Equal(t, "0143", errorResp.Code, "Error code should be 0143 for payload too large")
	assert.Equal(t, "Payload Too Large", errorResp.Title, "Error title should be Payload Too Large")
	assert.Equal(t, "payload too large: exceeds 100KB limit", errorResp.Message, "Error message should indicate payload size limit")
}
