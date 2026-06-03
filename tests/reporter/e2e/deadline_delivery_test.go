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

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/tests/reporter/e2e/shared"
)

// ############################################################################
// Delivery Tests (TC-DLV-001 to TC-DLV-004)
// ############################################################################

func TestDeadline_DeliverMarkAsDelivered(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a pending deadline
	input := map[string]any{
		"name":      shared.UniqueID("dl") + " deliver test",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDateDays(60),
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	shared.AssertDeadlineStatus(t, body, shared.DeadlineStatusPending)

	id := body["id"].(string)

	// Mark as delivered
	status, delivered, err := apiClient.DeliverDeadline(ctx, id, true)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertDeadlineStatus(t, delivered, shared.DeadlineStatusDelivered)

	// deliveredAt should be present
	require.Contains(t, delivered, "deliveredAt", "response should contain 'deliveredAt' after delivery")
}

func TestDeadline_DeliverClearDelivery(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create and deliver
	input := map[string]any{
		"name":      shared.UniqueID("dl") + " clear delivery",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDateDays(60),
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	id := body["id"].(string)

	// Deliver
	status, _, err = apiClient.DeliverDeadline(ctx, id, true)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	// Clear delivery
	status, cleared, err := apiClient.DeliverDeadline(ctx, id, false)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertDeadlineStatus(t, cleared, shared.DeadlineStatusPending)
}

func TestDeadline_DeliverAlreadyDelivered(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create and deliver
	input := map[string]any{
		"name":      shared.UniqueID("dl") + " idempotent deliver",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDateDays(60),
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	}

	status, body, err := apiClient.CreateDeadline(ctx, input)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)

	id := body["id"].(string)

	// First delivery
	status, _, err = apiClient.DeliverDeadline(ctx, id, true)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	// Second delivery — should be idempotent
	status, delivered, err := apiClient.DeliverDeadline(ctx, id, true)
	require.NoError(t, err)
	shared.AssertHTTPStatus(t, status, http.StatusOK)
	shared.AssertDeadlineStatus(t, delivered, shared.DeadlineStatusDelivered)
}
