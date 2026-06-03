// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"testing"
)

func TestDataSourceConfig_GetSchemas(t *testing.T) {
	// Note: Cannot use t.Parallel() because subtests use t.Setenv

	tests := []struct {
		name       string
		configName string
		envValue   string
		want       []string
	}{
		{
			name:       "returns configured schemas from environment",
			configName: "external_db",
			envValue:   "sales,inventory,reporting",
			want:       []string{"sales", "inventory", "reporting"},
		},
		{
			name:       "returns default public schema when not configured",
			configName: "midaz_onboarding",
			envValue:   "",
			want:       []string{"public"},
		},
		{
			name:       "returns single schema when only one configured",
			configName: "external_db",
			envValue:   "sales",
			want:       []string{"sales"},
		},
		{
			name:       "handles schema with spaces in comma-separated list",
			configName: "external_db",
			envValue:   "sales, inventory, reporting",
			want:       []string{"sales", "inventory", "reporting"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Note: Cannot use t.Parallel() because t.Setenv is used

			if tt.envValue != "" {
				envKey := "DATASOURCE_" + toEnvFormat(tt.configName) + "_SCHEMAS"
				t.Setenv(envKey, tt.envValue)
			}

			config := DataSourceConfig{
				ConfigName: tt.configName,
			}

			got := config.GetSchemas()

			if len(got) != len(tt.want) {
				t.Errorf("GetSchemas() returned %d schemas, want %d", len(got), len(tt.want))
				return
			}

			for i, schema := range got {
				if schema != tt.want[i] {
					t.Errorf("GetSchemas()[%d] = %q, want %q", i, schema, tt.want[i])
				}
			}
		})
	}
}

func TestDataSource_SchemasField(t *testing.T) {
	t.Parallel()

	ds := DataSource{
		DatabaseType: PostgreSQLType,
		Schemas:      []string{"sales", "inventory"},
	}

	if len(ds.Schemas) != 2 {
		t.Errorf("Schemas length = %d, want 2", len(ds.Schemas))
	}

	if ds.Schemas[0] != "sales" {
		t.Errorf("Schemas[0] = %q, want %q", ds.Schemas[0], "sales")
	}
}

// toEnvFormat converts a config name to environment variable format
// e.g., "external_db" -> "EXTERNAL_DB", "midaz-onboarding" -> "MIDAZ_ONBOARDING"
func toEnvFormat(name string) string {
	result := ""
	for _, c := range name {
		if c == '-' {
			result += "_"
		} else if c >= 'a' && c <= 'z' {
			result += string(c - 32) // Convert to uppercase
		} else {
			result += string(c)
		}
	}
	return result
}
