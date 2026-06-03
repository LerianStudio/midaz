// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterCondition_JSONMarshal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		filter   FilterCondition
		expected string
	}{
		{
			name: "Equals filter",
			filter: FilterCondition{
				Equals: []any{"active", "pending"},
			},
			expected: `{"eq":["active","pending"]}`,
		},
		{
			name: "GreaterThan filter",
			filter: FilterCondition{
				GreaterThan: []any{100},
			},
			expected: `{"gt":[100]}`,
		},
		{
			name: "Between filter",
			filter: FilterCondition{
				Between: []any{100, 1000},
			},
			expected: `{"between":[100,1000]}`,
		},
		{
			name: "In filter",
			filter: FilterCondition{
				In: []any{"active", "pending", "suspended"},
			},
			expected: `{"in":["active","pending","suspended"]}`,
		},
		{
			name: "NotIn filter",
			filter: FilterCondition{
				NotIn: []any{"deleted", "archived"},
			},
			expected: `{"nin":["deleted","archived"]}`,
		},
		{
			name:     "Empty filter",
			filter:   FilterCondition{},
			expected: `{}`,
		},
		{
			name: "Combined filters",
			filter: FilterCondition{
				GreaterOrEqual: []any{"2025-06-01"},
				LessOrEqual:    []any{"2025-06-30"},
			},
			expected: `{"gte":["2025-06-01"],"lte":["2025-06-30"]}`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(tt.filter)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(data))
		})
	}
}

func TestFilterCondition_JSONUnmarshal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		json     string
		expected FilterCondition
	}{
		{
			name: "Equals filter",
			json: `{"eq":["active"]}`,
			expected: FilterCondition{
				Equals: []any{"active"},
			},
		},
		{
			name: "GreaterThan with number",
			json: `{"gt":[100]}`,
			expected: FilterCondition{
				GreaterThan: []any{float64(100)}, // JSON numbers unmarshal as float64
			},
		},
		{
			name: "Between filter",
			json: `{"between":[100,500]}`,
			expected: FilterCondition{
				Between: []any{float64(100), float64(500)},
			},
		},
		{
			name: "Complex filter",
			json: `{"gte":["2025-01-01"],"lte":["2025-12-31"],"in":["active","pending"]}`,
			expected: FilterCondition{
				GreaterOrEqual: []any{"2025-01-01"},
				LessOrEqual:    []any{"2025-12-31"},
				In:             []any{"active", "pending"},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var result FilterCondition
			err := json.Unmarshal([]byte(tt.json), &result)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateReportInput_JSONMarshal(t *testing.T) {
	t.Parallel()
	input := CreateReportInput{
		TemplateID: "00000000-0000-0000-0000-000000000001",
		Filters: map[string]map[string]map[string]FilterCondition{
			"database": {
				"table": {
					"status": {
						Equals: []any{"active"},
					},
				},
			},
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "00000000-0000-0000-0000-000000000001", result["templateId"])
	assert.NotNil(t, result["filters"])
}

func TestCreateReportInput_JSONUnmarshal(t *testing.T) {
	t.Parallel()
	jsonData := `{
		"templateId": "00000000-0000-0000-0000-000000000001",
		"filters": {
			"midaz_onboarding": {
				"account": {
					"status": {"eq": ["active"]}
				}
			}
		}
	}`

	var input CreateReportInput
	err := json.Unmarshal([]byte(jsonData), &input)
	require.NoError(t, err)

	assert.Equal(t, "00000000-0000-0000-0000-000000000001", input.TemplateID)
	assert.NotNil(t, input.Filters)
	assert.NotNil(t, input.Filters["midaz_onboarding"])
	assert.NotNil(t, input.Filters["midaz_onboarding"]["account"])
	assert.NotNil(t, input.Filters["midaz_onboarding"]["account"]["status"])
}

func TestReportMessage_JSONMarshal(t *testing.T) {
	t.Parallel()
	templateID := uuid.New()
	reportID := uuid.New()

	msg := ReportMessage{
		TemplateID:   templateID,
		ReportID:     reportID,
		OutputFormat: "pdf",
		Filters: map[string]map[string]map[string]FilterCondition{
			"database": {
				"table": {
					"id": {
						In: []any{"1", "2", "3"},
					},
				},
			},
		},
		MappedFields: map[string]map[string][]string{
			"database": {
				"table": {"id", "name", "status"},
			},
		},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, templateID.String(), result["templateId"])
	assert.Equal(t, reportID.String(), result["reportId"])
	assert.Equal(t, "pdf", result["outputFormat"])
}

func TestReportMessage_JSONUnmarshal(t *testing.T) {
	t.Parallel()
	templateID := uuid.New()
	reportID := uuid.New()

	jsonData := `{
		"templateId": "` + templateID.String() + `",
		"reportId": "` + reportID.String() + `",
		"outputFormat": "html",
		"filters": {
			"db": {
				"users": {
					"age": {"gte": [18]}
				}
			}
		},
		"mappedFields": {
			"db": {
				"users": ["id", "name", "email"]
			}
		}
	}`

	var msg ReportMessage
	err := json.Unmarshal([]byte(jsonData), &msg)
	require.NoError(t, err)

	assert.Equal(t, templateID, msg.TemplateID)
	assert.Equal(t, reportID, msg.ReportID)
	assert.Equal(t, "html", msg.OutputFormat)
	assert.NotNil(t, msg.Filters)
	assert.NotNil(t, msg.MappedFields)
}

func TestFilterCondition_AllOperators(t *testing.T) {
	t.Parallel()
	filter := FilterCondition{
		Equals:         []any{"value1"},
		GreaterThan:    []any{10},
		GreaterOrEqual: []any{5},
		LessThan:       []any{100},
		LessOrEqual:    []any{50},
		Between:        []any{1, 10},
		In:             []any{"a", "b"},
		NotIn:          []any{"x", "y"},
	}

	data, err := json.Marshal(filter)
	require.NoError(t, err)

	var result FilterCondition
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.NotNil(t, result.Equals)
	assert.NotNil(t, result.GreaterThan)
	assert.NotNil(t, result.GreaterOrEqual)
	assert.NotNil(t, result.LessThan)
	assert.NotNil(t, result.LessOrEqual)
	assert.NotNil(t, result.Between)
	assert.NotNil(t, result.In)
	assert.NotNil(t, result.NotIn)
}

func TestReportMessage_EmptyFilters(t *testing.T) {
	t.Parallel()
	msg := ReportMessage{
		TemplateID:   uuid.New(),
		ReportID:     uuid.New(),
		OutputFormat: "xml",
		Filters:      nil,
		MappedFields: nil,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var result ReportMessage
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Nil(t, result.Filters)
	assert.Nil(t, result.MappedFields)
}

func TestNewCreateReportInput(t *testing.T) {
	t.Parallel()

	validUUID := "00000000-0000-0000-0000-000000000001"
	validFilters := map[string]map[string]map[string]FilterCondition{
		"database": {
			"table": {
				"status": {Equals: []any{"active"}},
			},
		},
	}

	tests := []struct {
		name        string
		templateID  string
		filters     map[string]map[string]map[string]FilterCondition
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid input with filters",
			templateID: validUUID,
			filters:    validFilters,
			wantErr:    false,
		},
		{
			name:       "valid input with nil filters",
			templateID: validUUID,
			filters:    nil,
			wantErr:    false,
		},
		{
			name:       "valid input with empty filters",
			templateID: validUUID,
			filters:    map[string]map[string]map[string]FilterCondition{},
			wantErr:    false,
		},
		{
			name:        "empty templateID",
			templateID:  "",
			filters:     validFilters,
			wantErr:     true,
			errContains: "templateID must not be empty",
		},
		{
			name:        "invalid UUID format - plain string",
			templateID:  "not-a-uuid",
			filters:     validFilters,
			wantErr:     true,
			errContains: "templateID must be a valid UUID",
		},
		{
			name:        "invalid UUID format - too short",
			templateID:  "12345",
			filters:     validFilters,
			wantErr:     true,
			errContains: "templateID must be a valid UUID",
		},
		{
			name:        "invalid UUID format - wrong separators",
			templateID:  "00000000_0000_0000_0000_000000000001",
			filters:     validFilters,
			wantErr:     true,
			errContains: "templateID must be a valid UUID",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := NewCreateReportInput(tt.templateID, tt.filters)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.templateID, result.TemplateID)
				assert.Equal(t, tt.filters, result.Filters)
			}
		})
	}
}

func TestNewReportMessage(t *testing.T) {
	t.Parallel()

	validTemplateID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	validReportID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	validOutputFormat := "pdf"
	validMappedFields := map[string]map[string][]string{
		"database": {
			"table": {"id", "name", "status"},
		},
	}
	validFilters := map[string]map[string]map[string]FilterCondition{
		"database": {
			"table": {
				"status": {Equals: []any{"active"}},
			},
		},
	}

	tests := []struct {
		name         string
		templateID   uuid.UUID
		reportID     uuid.UUID
		outputFormat string
		mappedFields map[string]map[string][]string
		filters      map[string]map[string]map[string]FilterCondition
		wantErr      bool
		errContains  string
	}{
		{
			name:         "valid input with all fields",
			templateID:   validTemplateID,
			reportID:     validReportID,
			outputFormat: validOutputFormat,
			mappedFields: validMappedFields,
			filters:      validFilters,
			wantErr:      false,
		},
		{
			name:         "valid input with nil mappedFields and filters",
			templateID:   validTemplateID,
			reportID:     validReportID,
			outputFormat: "html",
			mappedFields: nil,
			filters:      nil,
			wantErr:      false,
		},
		{
			name:         "nil templateID",
			templateID:   uuid.Nil,
			reportID:     validReportID,
			outputFormat: validOutputFormat,
			mappedFields: validMappedFields,
			filters:      validFilters,
			wantErr:      true,
			errContains:  "templateID must not be nil",
		},
		{
			name:         "nil reportID",
			templateID:   validTemplateID,
			reportID:     uuid.Nil,
			outputFormat: validOutputFormat,
			mappedFields: validMappedFields,
			filters:      validFilters,
			wantErr:      true,
			errContains:  "reportID must not be nil",
		},
		{
			name:         "empty outputFormat",
			templateID:   validTemplateID,
			reportID:     validReportID,
			outputFormat: "",
			mappedFields: validMappedFields,
			filters:      validFilters,
			wantErr:      true,
			errContains:  "outputFormat must not be empty",
		},
		{
			name:         "both templateID and reportID nil",
			templateID:   uuid.Nil,
			reportID:     uuid.Nil,
			outputFormat: validOutputFormat,
			mappedFields: nil,
			filters:      nil,
			wantErr:      true,
			errContains:  "templateID must not be nil",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := NewReportMessage(tt.templateID, tt.reportID, tt.outputFormat, tt.mappedFields, tt.filters)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.templateID, result.TemplateID)
				assert.Equal(t, tt.reportID, result.ReportID)
				assert.Equal(t, tt.outputFormat, result.OutputFormat)
				assert.Equal(t, tt.mappedFields, result.MappedFields)
				assert.Equal(t, tt.filters, result.Filters)
			}
		})
	}
}
