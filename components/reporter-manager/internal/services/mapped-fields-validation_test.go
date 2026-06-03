// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUseCase_TransformMappedFieldsForStorage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		mappedFields   map[string]map[string][]string
		organizationID string
		expected       map[string]map[string][]string
	}{
		{
			name: "Success - Regular database no transformation",
			mappedFields: map[string]map[string][]string{
				"midaz_onboarding": {
					"users":    {"id", "name"},
					"accounts": {"id", "balance"},
				},
			},
			organizationID: "org-123",
			expected: map[string]map[string][]string{
				"midaz_onboarding": {
					"users":    {"id", "name"},
					"accounts": {"id", "balance"},
				},
			},
		},
		{
			name: "Success - Plugin CRM database adds organization mapping",
			mappedFields: map[string]map[string][]string{
				"plugin_crm": {
					"contacts": {"id", "name"},
				},
			},
			organizationID: "org-456",
			expected: map[string]map[string][]string{
				"plugin_crm": {
					"contacts":     {"id", "name"},
					"organization": {"org-456"},
				},
			},
		},
		{
			name: "Success - Multiple databases mixed transformation",
			mappedFields: map[string]map[string][]string{
				"midaz_onboarding": {
					"users": {"id"},
				},
				"plugin_crm": {
					"leads": {"name", "email"},
				},
			},
			organizationID: "org-789",
			expected: map[string]map[string][]string{
				"midaz_onboarding": {
					"users": {"id"},
				},
				"plugin_crm": {
					"leads":        {"name", "email"},
					"organization": {"org-789"},
				},
			},
		},
		{
			name:           "Success - Empty mapped fields",
			mappedFields:   map[string]map[string][]string{},
			organizationID: "org-123",
			expected:       map[string]map[string][]string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := TransformMappedFieldsForStorage(tt.mappedFields, tt.organizationID)

			// Check database count
			assert.Equal(t, len(tt.expected), len(result))

			// Check each database's tables
			for dbName, expectedTables := range tt.expected {
				resultTables, exists := result[dbName]
				require.True(t, exists, "Expected database %s to exist in result", dbName)
				assert.Equal(t, len(expectedTables), len(resultTables), "Table count mismatch for database %s", dbName)

				for tableName, expectedFields := range expectedTables {
					resultFields, tableExists := resultTables[tableName]
					require.True(t, tableExists, "Expected table %s to exist in database %s", tableName, dbName)
					assert.ElementsMatch(t, expectedFields, resultFields, "Fields mismatch for table %s in database %s", tableName, dbName)
				}
			}
		})
	}
}
