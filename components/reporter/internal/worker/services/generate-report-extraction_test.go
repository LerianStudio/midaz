// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"encoding/json"
	"testing"

	fetcherEngine "github.com/LerianStudio/fetcher/pkg/engine"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToEngineTableKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "public.organization", toEngineTableKey("public__organization"))
	assert.Equal(t, "public.accounts", toEngineTableKey("public__accounts"))
	assert.Equal(t, "organization", toEngineTableKey("organization"), "bare key is unchanged")
}

// snapshotOf builds a SchemaSnapshot from qualified table names so the resolver
// tests assert against the shape the REAL postgres connector emits ("schema.table").
func snapshotOf(qualifiedTables ...string) fetcherEngine.SchemaSnapshot {
	tables := make([]fetcherEngine.TableSnapshot, 0, len(qualifiedTables))
	for _, name := range qualifiedTables {
		tables = append(tables, fetcherEngine.TableSnapshot{Name: name})
	}

	return fetcherEngine.SchemaSnapshot{Tables: tables}
}

// TestResolveTableKeys covers the schema-autodiscovery parity the legacy
// SchemaResolver provided: bare postgres keys must qualify against the REAL
// connector snapshot shape (qualified "schema.table"), explicit keys must match
// exactly, ambiguous bare keys must error with guidance, and a missing key must
// fail loudly rather than silently drop the section.
func TestResolveTableKeys(t *testing.T) {
	t.Parallel()

	t.Run("bare postgres key autodiscovers single schema", func(t *testing.T) {
		t.Parallel()

		keyMap, err := resolveTableKeys("onboarding",
			snapshotOf("public.organization", "public.ledger"),
			map[string][]string{"organization": {"name"}})
		require.NoError(t, err)
		assert.Equal(t, "public.organization", keyMap["organization"],
			"bare key resolves to its qualified snapshot table")
	})

	t.Run("bare key in non-public schema autodiscovers that schema", func(t *testing.T) {
		t.Parallel()

		keyMap, err := resolveTableKeys("onboarding",
			snapshotOf("audit.events"),
			map[string][]string{"events": {"id"}})
		require.NoError(t, err)
		assert.Equal(t, "audit.events", keyMap["events"])
	})

	t.Run("bare key in multiple schemas prefers public", func(t *testing.T) {
		t.Parallel()

		keyMap, err := resolveTableKeys("onboarding",
			snapshotOf("public.accounts", "archive.accounts"),
			map[string][]string{"accounts": {"id"}})
		require.NoError(t, err)
		assert.Equal(t, "public.accounts", keyMap["accounts"])
	})

	t.Run("ambiguous bare key without public errors with guidance", func(t *testing.T) {
		t.Parallel()

		_, err := resolveTableKeys("onboarding",
			snapshotOf("archive.accounts", "staging.accounts"),
			map[string][]string{"accounts": {"id"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "multiple schemas")
		assert.Contains(t, err.Error(), "archive__accounts")
	})

	t.Run("explicit Pongo2 key matches exactly", func(t *testing.T) {
		t.Parallel()

		keyMap, err := resolveTableKeys("onboarding",
			snapshotOf("audit.organization"),
			map[string][]string{"audit__organization": {"name"}})
		require.NoError(t, err)
		assert.Equal(t, "audit.organization", keyMap["audit__organization"])
	})

	t.Run("explicit key absent from snapshot fails", func(t *testing.T) {
		t.Parallel()

		_, err := resolveTableKeys("onboarding",
			snapshotOf("public.organization"),
			map[string][]string{"audit__organization": {"name"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("missing bare key fails loudly", func(t *testing.T) {
		t.Parallel()

		_, err := resolveTableKeys("onboarding",
			snapshotOf("public.organization"),
			map[string][]string{"ledger": {"id"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("bare mongo collection matches verbatim", func(t *testing.T) {
		t.Parallel()

		keyMap, err := resolveTableKeys("crm",
			snapshotOf("holders"),
			map[string][]string{"holders": {"name"}})
		require.NoError(t, err)
		assert.Equal(t, "holders", keyMap["holders"], "bare collection resolves to itself")
	})
}

// TestRekeyEngineResult proves the parity-critical re-keying: the engine
// aggregates rows under config name then the qualified table it queried, and the
// renderer consumes map[databaseName][originalTemplateKey][]rows. The keyMap
// (original -> engine) is inverted so each engine result is stored under the
// ORIGINAL template key — bare when the template used a bare reference, qualified
// when explicit.
func TestRekeyEngineResult(t *testing.T) {
	t.Parallel()

	t.Run("bare-keyed template gets rows back under the bare key", func(t *testing.T) {
		t.Parallel()

		decoded := map[string]map[string][]map[string]any{
			"onboarding": {
				"public.organization": {{"name": "World"}},
				"public.ledger":       {{"id": "L1"}},
			},
		}
		keyMap := map[string]string{
			"organization": "public.organization",
			"ledger":       "public.ledger",
		}
		result := make(map[string]map[string][]map[string]any)

		rekeyEngineResult("onboarding", decoded, keyMap, result)

		require.Contains(t, result, "onboarding")
		assert.Equal(t, []map[string]any{{"name": "World"}}, result["onboarding"]["organization"],
			"bare template key, not public__organization")
		assert.Equal(t, []map[string]any{{"id": "L1"}}, result["onboarding"]["ledger"])
	})

	t.Run("explicit Pongo2 template key is preserved", func(t *testing.T) {
		t.Parallel()

		decoded := map[string]map[string][]map[string]any{
			"onboarding": {"audit.organization": {{"name": "World"}}},
		}
		keyMap := map[string]string{"audit__organization": "audit.organization"}
		result := make(map[string]map[string][]map[string]any)

		rekeyEngineResult("onboarding", decoded, keyMap, result)

		assert.Equal(t, []map[string]any{{"name": "World"}}, result["onboarding"]["audit__organization"])
	})

	t.Run("bare collection key is unchanged", func(t *testing.T) {
		t.Parallel()

		decoded := map[string]map[string][]map[string]any{
			"crm": {"holders": {{"id": "H1"}}},
		}
		keyMap := map[string]string{"holders": "holders"}
		result := make(map[string]map[string][]map[string]any)

		rekeyEngineResult("crm", decoded, keyMap, result)

		assert.Equal(t, []map[string]any{{"id": "H1"}}, result["crm"]["holders"])
	})

	t.Run("merges into an existing section", func(t *testing.T) {
		t.Parallel()

		decoded := map[string]map[string][]map[string]any{
			"onboarding": {"public.accounts": {{"id": "A1"}}},
		}
		keyMap := map[string]string{"accounts": "public.accounts"}
		result := map[string]map[string][]map[string]any{
			"onboarding": {"existing": {{"id": "E1"}}},
		}

		rekeyEngineResult("onboarding", decoded, keyMap, result)

		assert.Len(t, result["onboarding"], 2)
		assert.Equal(t, []map[string]any{{"id": "E1"}}, result["onboarding"]["existing"])
		assert.Equal(t, []map[string]any{{"id": "A1"}}, result["onboarding"]["accounts"])
	})

	t.Run("missing datasource key is a no-op", func(t *testing.T) {
		t.Parallel()

		result := make(map[string]map[string][]map[string]any)
		rekeyEngineResult("absent", map[string]map[string][]map[string]any{"other": {}}, nil, result)
		assert.Empty(t, result)
	})
}

func TestDecodeDirectResult(t *testing.T) {
	t.Parallel()

	t.Run("decodes nested payload", func(t *testing.T) {
		t.Parallel()

		payload, err := json.Marshal(map[string]map[string][]map[string]any{
			"onboarding": {"public.organization": {{"name": "World"}}},
		})
		require.NoError(t, err)

		decoded, err := decodeDirectResult(fetcherEngine.ExtractionResult{
			Direct: &fetcherEngine.DirectResult{Data: payload},
		})
		require.NoError(t, err)
		assert.Equal(t, []map[string]any{{"name": "World"}}, decoded["onboarding"]["public.organization"])
	})

	t.Run("empty data yields empty map", func(t *testing.T) {
		t.Parallel()

		decoded, err := decodeDirectResult(fetcherEngine.ExtractionResult{
			Direct: &fetcherEngine.DirectResult{},
		})
		require.NoError(t, err)
		assert.Empty(t, decoded)
	})

	t.Run("nil direct result fails", func(t *testing.T) {
		t.Parallel()

		_, err := decodeDirectResult(fetcherEngine.ExtractionResult{})
		require.Error(t, err)
	})

	t.Run("malformed json fails", func(t *testing.T) {
		t.Parallel()

		_, err := decodeDirectResult(fetcherEngine.ExtractionResult{
			Direct: &fetcherEngine.DirectResult{Data: []byte("{not json")},
		})
		require.Error(t, err)
	})
}

// TestBuildExtractionRequest proves the request carries the dot-notation field
// selection and the nested map[string]any filter shape the engine planner walks.
func TestBuildExtractionRequest(t *testing.T) {
	t.Parallel()

	reportID := uuid.New()
	templateID := uuid.New()
	condition := model.FilterCondition{Equals: []any{"World"}}

	message := GenerateReportMessage{
		ReportID:   reportID,
		TemplateID: templateID,
		DataQueries: map[string]map[string][]string{
			"onboarding": {"public__organization": {"name"}},
		},
		Filters: map[string]map[string]map[string]model.FilterCondition{
			"onboarding": {
				"public__organization": {"name": condition},
			},
		},
	}

	keyMap := map[string]string{"public__organization": "public.organization"}
	req := buildExtractionRequest(message, "onboarding", message.DataQueries["onboarding"], keyMap)

	// MappedFields are dot-notation, keyed by datasource config name.
	require.Contains(t, req.MappedFields, "onboarding")
	assert.Equal(t, []string{"name"}, req.MappedFields["onboarding"]["public.organization"])

	// Filters are the nested map[string]any the planner walks, with the leaf
	// staying a typed FilterCondition and the table key in dot-notation.
	dsFilters, ok := req.Filters["onboarding"].(map[string]any)
	require.True(t, ok)
	tableFilters, ok := dsFilters["public.organization"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, condition, tableFilters["name"])

	// Metadata carries safe routing fields, no secrets.
	assert.Equal(t, reportID.String(), req.Metadata["reportId"])
	assert.Equal(t, templateID.String(), req.Metadata["templateId"])
}
