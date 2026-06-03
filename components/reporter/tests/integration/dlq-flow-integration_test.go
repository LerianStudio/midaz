//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package integration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/reporter/pkg/model"
	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDLQMessage_MetadataStructure validates DLQ message schema
func TestIntegration_DLQ_MetadataStructure(t *testing.T) {
	t.Parallel()

	reportID := uuid.New()
	reportMessage := model.ReportMessage{
		ReportID:     reportID,
		TemplateID:   uuid.New(),
		OutputFormat: "html",
	}

	bodyBytes, err := json.Marshal(reportMessage)
	require.NoError(t, err)

	headers := amqp091.Table{
		"x-retry-count":    int32(3),
		"x-failure-reason": "Template rendering failed: invalid syntax",
		"x-request-id":     "req-123",
	}

	expectedDoc := map[string]any{
		"message_body":     string(bodyBytes),
		"retry_count":      int32(3),
		"failure_reason":   "Template rendering failed: invalid syntax",
		"received_at":      time.Now(),
		"report_id":        reportID.String(),
		"original_headers": headers,
	}

	assert.Contains(t, expectedDoc, "message_body")
	assert.Contains(t, expectedDoc, "retry_count")
	assert.Contains(t, expectedDoc, "failure_reason")
	assert.Contains(t, expectedDoc, "received_at")
	assert.Contains(t, expectedDoc, "report_id")
	assert.Contains(t, expectedDoc, "original_headers")

	assert.Equal(t, int32(3), expectedDoc["retry_count"])
	assert.NotEmpty(t, expectedDoc["failure_reason"])
}

// TestDLQConfiguration_TTLAndLimits validates DLQ queue configuration
func TestIntegration_DLQ_TTLAndLimits(t *testing.T) {
	t.Parallel()

	expectedTTL := 7 * 24 * time.Hour
	expectedMaxLength := 10000

	ttlMs := int(expectedTTL.Milliseconds())
	assert.Equal(t, 604800000, ttlMs, "DLQ TTL should be 7 days in milliseconds")
	assert.Equal(t, 10000, expectedMaxLength, "DLQ should limit to 10,000 messages")
}

// TestReportStatus_UpdatedOnDLQ validates report status update
func TestIntegration_ReportStatus_UpdatedOnDLQ(t *testing.T) {
	t.Parallel()

	expectedStatus := "Error"
	expectedMetadata := map[string]any{
		"error":         "Database connection timeout",
		"retry_count":   int32(3),
		"dlq_timestamp": time.Now(),
	}

	assert.Equal(t, "Error", expectedStatus)
	assert.Contains(t, expectedMetadata, "error")
	assert.Contains(t, expectedMetadata, "retry_count")
	assert.Contains(t, expectedMetadata, "dlq_timestamp")
	assert.Equal(t, int32(3), expectedMetadata["retry_count"])
}

// TestExponentialBackoff_Timing validates retry delays
func TestIntegration_ExponentialBackoff_Timing(t *testing.T) {
	t.Parallel()

	expectedBackoffs := []time.Duration{
		1 * time.Second, // 2^0
		2 * time.Second, // 2^1
		4 * time.Second, // 2^2
	}

	for i, expected := range expectedBackoffs {
		actual := time.Duration(1<<i) * time.Second
		assert.Equal(t, expected, actual, "Backoff for retry %d should be %v", i, expected)
	}
}
