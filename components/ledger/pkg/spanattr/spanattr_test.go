// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package spanattr

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestSafePayloadAttributes(t *testing.T) {
	t.Parallel()

	alias := "@ops"
	portfolioID := "portfolio-id"
	segmentID := "segment-id"
	entityID := "entity-id"

	tests := []struct {
		name      string
		payload   any
		assertion func(t *testing.T, attrs map[string]any)
	}{
		{
			name: "account payload only exposes safe presence flags",
			payload: &mmodel.CreateAccountInput{
				Alias:       &alias,
				PortfolioID: &portfolioID,
				SegmentID:   &segmentID,
				EntityID:    &entityID,
				Metadata:    map[string]any{"secret": "value"},
			},
			assertion: func(t *testing.T, attrs map[string]any) {
				t.Helper()

				assert.Equal(t, "CreateAccountInput", attrs["app.request.payload.type"])
				assert.Equal(t, true, attrs["app.request.payload.has_metadata"])
				assert.Equal(t, true, attrs["app.request.payload.has_portfolio_id"])
				assert.Equal(t, true, attrs["app.request.payload.has_segment_id"])
				assert.Equal(t, true, attrs["app.request.payload.has_entity_id"])
				assert.Equal(t, true, attrs["app.request.payload.has_alias"])
			},
		},
		{
			name:    "nil payload is reported as unknown without extra data",
			payload: nil,
			assertion: func(t *testing.T, attrs map[string]any) {
				t.Helper()

				assert.Equal(t, "unknown", attrs["app.request.payload.type"])
				assert.Equal(t, false, attrs["app.request.payload.has_metadata"])
				assert.Equal(t, false, attrs["app.request.payload.has_alias"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			attrs := attributeMap(SafePayloadAttributes(tt.payload))
			tt.assertion(t, attrs)
		})
	}
}

func TestSafePayloadAttributes_RedactsValues(t *testing.T) {
	t.Parallel()

	payload := struct {
		Alias          string
		Key            string
		OrganizationID string
	}{
		Alias:          "sensitive-alias",
		Key:            "super-secret-key",
		OrganizationID: "org-123",
	}

	attrs := SafePayloadAttributes(payload)
	require.NotEmpty(t, attrs)

	for _, attr := range attrs {
		serialized := fmt.Sprint(attr.Value.AsInterface())
		assert.NotContains(t, serialized, "sensitive-alias")
		assert.NotContains(t, serialized, "super-secret-key")
		assert.NotContains(t, serialized, "org-123")
	}
}

func attributeMap(attrs []attribute.KeyValue) map[string]any {
	result := make(map[string]any, len(attrs))

	for _, attr := range attrs {
		switch attr.Value.Type() {
		case attribute.BOOL:
			result[string(attr.Key)] = attr.Value.AsBool()
		case attribute.INT64:
			result[string(attr.Key)] = attr.Value.AsInt64()
		case attribute.STRING:
			result[string(attr.Key)] = attr.Value.AsString()
		default:
			result[string(attr.Key)] = attr.Value.Emit()
		}
	}

	return result
}
