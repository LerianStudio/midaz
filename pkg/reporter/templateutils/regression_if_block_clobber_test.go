// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package templateutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMappedFieldsOfTemplate_IfBlockDoesNotClobberDatasourceMap is a regression
// test for a parser bug where {% if datasource.field %} processed AFTER a
// {% for x in datasource.table %}…{{ x.f }}{% endfor %} would overwrite the
// populated table map at insertField's last-iteration default branch.
// normalizeStructure then dropped the resulting []any silently, leaving the
// inner map empty, and ValidateSchemaViaProvider would skip the datasource
// entirely (its `if len(tables) == 0 { continue }` short-circuit).
//
// Net effect before the fix: a template referencing a non-existent datasource
// (e.g. "marib_apelido") was accepted by POST /templates without any schema
// error — neither ErrMissingDataSource nor ErrMissingSchemaTable was raised.
//
// Fix: insertField now preserves an existing map[string]any at the final
// position of a 1-element path and intentionally drops the orphan field
// reference, so the populated tables survive AND the validator still has a
// non-empty inner map for the datasource (skipping the len(tables)==0
// short-circuit and surfacing ErrMissingDataSource when appropriate).
func TestMappedFieldsOfTemplate_IfBlockDoesNotClobberDatasourceMap(t *testing.T) {
	t.Parallel()

	tpl := `<!DOCTYPE html>
<html>
<body>
    {% if marib_apelido.nao_existe %}
        {% for a in marib_apelido.account %}
            <td>{{ a.id }}</td>
            <td>{{ a.name }}</td>
            <td>{{ a.alias }}</td>
        {% endfor %}
    {% else %}
        <p>No accounts found.</p>
    {% endif %}
</body>
</html>`

	result := MappedFieldsOfTemplate(tpl)

	require.NotNil(t, result)
	require.Contains(t, result, "marib_apelido",
		"datasource must be registered so the schema validator can check its existence")

	ds := result["marib_apelido"]

	require.Contains(t, ds, "account",
		"the table populated by the {%% for %%} loop must survive the trailing {%% if %%}")
	assert.ElementsMatch(t, []string{"id", "name", "alias"}, ds["account"],
		"all fields referenced inside the {%% for %%} loop must be preserved")

	// The inner map must be non-empty so ValidateSchemaViaProvider does NOT
	// take its `len(tables) == 0 { continue }` early-exit and actually asks
	// the DataSourceProvider whether "marib_apelido" exists. With the
	// "account" table preserved this is guaranteed and the validator will
	// raise ErrMissingDataSource (or the correct schema error) instead of
	// accepting the template silently.
	assert.NotEmpty(t, ds,
		"inner table map must be non-empty so schema validation is not skipped")
}
