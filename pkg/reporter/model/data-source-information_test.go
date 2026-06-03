// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataSourceInformation_JSONMarshal(t *testing.T) {
	t.Parallel()
	info := DataSourceInformation{
		Id:           "midaz_onboarding",
		ExternalName: "onboarding",
		Type:         "postgresql",
	}

	data, err := json.Marshal(info)
	require.NoError(t, err)

	expected := `{"id":"midaz_onboarding","externalName":"onboarding","type":"postgresql"}`
	assert.JSONEq(t, expected, string(data))
}

func TestDataSourceInformation_JSONUnmarshal(t *testing.T) {
	t.Parallel()
	jsonData := `{"id":"test_db","externalName":"test","type":"mongodb"}`

	var info DataSourceInformation
	err := json.Unmarshal([]byte(jsonData), &info)
	require.NoError(t, err)

	assert.Equal(t, "test_db", info.Id)
	assert.Equal(t, "test", info.ExternalName)
	assert.Equal(t, "mongodb", info.Type)
}

func TestDataSourceDetails_JSONMarshal(t *testing.T) {
	t.Parallel()
	details := DataSourceDetails{
		Id:           "midaz_onboarding",
		ExternalName: "onboarding",
		Type:         "postgresql",
		Tables: []TableDetails{
			{
				Name:   "account",
				Fields: []string{"id", "name", "parent_account_id"},
			},
			{
				Name:   "organization",
				Fields: []string{"id", "legal_name", "legal_document"},
			},
		},
	}

	data, err := json.Marshal(details)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "midaz_onboarding", result["id"])
	assert.Equal(t, "onboarding", result["externalName"])
	assert.Equal(t, "postgresql", result["type"])

	tables, ok := result["tables"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, tables, 2)
}

func TestDataSourceDetails_JSONUnmarshal(t *testing.T) {
	t.Parallel()
	jsonData := `{
		"id": "test_db",
		"externalName": "test",
		"type": "postgresql",
		"tables": [
			{
				"name": "users",
				"fields": ["id", "email", "name"]
			}
		]
	}`

	var details DataSourceDetails
	err := json.Unmarshal([]byte(jsonData), &details)
	require.NoError(t, err)

	assert.Equal(t, "test_db", details.Id)
	assert.Equal(t, "test", details.ExternalName)
	assert.Equal(t, "postgresql", details.Type)
	assert.Len(t, details.Tables, 1)
	assert.Equal(t, "users", details.Tables[0].Name)
	assert.Equal(t, []string{"id", "email", "name"}, details.Tables[0].Fields)
}

func TestTableDetails_JSONMarshal(t *testing.T) {
	t.Parallel()
	table := TableDetails{
		Name:   "account",
		Fields: []string{"id", "name", "type", "status"},
	}

	data, err := json.Marshal(table)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "account", result["name"])
	fields, ok := result["fields"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, fields, 4)
}

func TestTableDetails_JSONUnmarshal(t *testing.T) {
	t.Parallel()
	jsonData := `{"name":"transactions","fields":["id","amount","currency","created_at"]}`

	var table TableDetails
	err := json.Unmarshal([]byte(jsonData), &table)
	require.NoError(t, err)

	assert.Equal(t, "transactions", table.Name)
	assert.Equal(t, []string{"id", "amount", "currency", "created_at"}, table.Fields)
}

func TestDataSourceDetails_EmptyTables(t *testing.T) {
	t.Parallel()
	details := DataSourceDetails{
		Id:           "empty_db",
		ExternalName: "empty",
		Type:         "mongodb",
		Tables:       []TableDetails{},
	}

	data, err := json.Marshal(details)
	require.NoError(t, err)

	var result DataSourceDetails
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "empty_db", result.Id)
	assert.Empty(t, result.Tables)
}

func TestTableDetails_EmptyFields(t *testing.T) {
	t.Parallel()
	table := TableDetails{
		Name:   "empty_table",
		Fields: []string{},
	}

	data, err := json.Marshal(table)
	require.NoError(t, err)

	var result TableDetails
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "empty_table", result.Name)
	assert.Empty(t, result.Fields)
}

func TestDataSourceInformation_AllTypes(t *testing.T) {
	t.Parallel()
	types := []string{"postgresql", "mongodb", "mysql", "mariadb"}

	for _, dbType := range types {
		dbType := dbType
		t.Run("Success - Type_"+dbType, func(t *testing.T) {
			t.Parallel()
			info := DataSourceInformation{
				Id:           "test_" + dbType,
				ExternalName: dbType,
				Type:         dbType,
			}

			data, err := json.Marshal(info)
			require.NoError(t, err)

			var result DataSourceInformation
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			assert.Equal(t, dbType, result.Type)
		})
	}
}
