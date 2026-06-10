// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

// TransformMappedFieldsForStorage transforms mapped fields for storage in the database.
// For crm database, an organization mapping entry is added when organizationID is provided.
func TransformMappedFieldsForStorage(mappedFields map[string]map[string][]string, organizationID string) map[string]map[string][]string {
	transformedFields := make(map[string]map[string][]string)

	for databaseName, tables := range mappedFields {
		transformedTables := make(map[string][]string)

		for tableName, fields := range tables {
			transformedTables[tableName] = fields
		}

		// For crm database, add organization mapping only if organizationID is provided
		if databaseName == crmDataSourceID && organizationID != "" {
			transformedTables["organization"] = []string{organizationID}
		}

		transformedFields[databaseName] = transformedTables
	}

	return transformedFields
}
