// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	libHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/attribute"
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

			attrs := attributeMap(safePayloadAttributes(tt.payload))
			tt.assertion(t, attrs)
		})
	}
}

func TestSafePayloadSummary_RedactsValues(t *testing.T) {
	t.Parallel()

	alias := "@customer-sensitive"
	payload := &mmodel.CreateAccountInput{
		Alias:    &alias,
		Metadata: map[string]any{"taxId": "sensitive"},
	}

	summary := safePayloadSummary(payload)

	assert.Contains(t, summary, "type=CreateAccountInput")
	assert.Contains(t, summary, "hasMetadata=true")
	assert.Contains(t, summary, "hasAlias=true")
	assert.NotContains(t, summary, alias)
	assert.NotContains(t, summary, "sensitive")
}

func TestSafePayloadSummary_RedactsLegalDocumentValues(t *testing.T) {
	t.Parallel()

	legalDocument := "12345678901234"
	payload := &mmodel.CreateOrganizationInput{
		LegalDocument: legalDocument,
		Metadata:      map[string]any{"taxId": "sensitive"},
	}

	summary := safePayloadSummary(payload)

	assert.Contains(t, summary, "type=CreateOrganizationInput")
	assert.Contains(t, summary, "hasMetadata=true")
	assert.Contains(t, summary, "hasLegalDocument=true")
	assert.NotContains(t, summary, legalDocument)
	assert.NotContains(t, summary, "sensitive")
}

func TestSafeQueryAttributes(t *testing.T) {
	t.Parallel()

	query := &libHTTP.QueryHeader{
		Limit:        25,
		Page:         3,
		Cursor:       "secret-cursor-token",
		SortOrder:    "desc",
		Metadata:     &bson.M{"private": "value"},
		PortfolioID:  ptr("portfolio-id"),
		HolderID:     ptr("holder-id"),
		Document:     ptr("1234567890"),
		ToAssetCodes: []string{"USD", "BRL"},
		StartDate:    time.Date(2026, time.March, 12, 0, 0, 0, 0, time.UTC),
	}

	attrs := attributeMap(safeQueryAttributes(query))

	require.Equal(t, true, attrs["app.request.query.present"])
	assert.Equal(t, int64(25), attrs["app.request.query.limit"])
	assert.Equal(t, int64(3), attrs["app.request.query.page"])
	assert.Equal(t, "desc", attrs["app.request.query.sort_order"])
	assert.Equal(t, true, attrs["app.request.query.has_cursor"])
	assert.Equal(t, true, attrs["app.request.query.has_metadata"])
	assert.Equal(t, true, attrs["app.request.query.has_date_range"])
	assert.Equal(t, true, attrs["app.request.query.has_portfolio_id"])
	assert.Equal(t, true, attrs["app.request.query.has_holder_id"])
	assert.Equal(t, true, attrs["app.request.query.has_document"])
	assert.Equal(t, int64(2), attrs["app.request.query.to_asset_codes_count"])
	assert.NotContains(t, attrs, "app.request.query.cursor")
	assert.NotContains(t, attrs, "app.request.query.metadata")
	assert.NotContains(t, attrs, "app.request.query.document")
}

func TestSafeQueryAttributes_DefaultAndNilCases(t *testing.T) {
	t.Parallel()

	nilAttrs := attributeMap(safeQueryAttributes(nil))
	require.Equal(t, false, nilAttrs["app.request.query.present"])

	defaultAttrs := attributeMap(safeQueryAttributes(&libHTTP.QueryHeader{}))
	require.Equal(t, true, defaultAttrs["app.request.query.present"])
	assert.Equal(t, "", defaultAttrs["app.request.query.sort_order"])
	assert.Equal(t, false, defaultAttrs["app.request.query.has_cursor"])
	assert.Equal(t, int64(0), defaultAttrs["app.request.query.to_asset_codes_count"])
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

func ptr[T any](value T) *T {
	return &value
}
