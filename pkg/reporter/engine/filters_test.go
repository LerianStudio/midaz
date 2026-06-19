// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateFilterField_RejectsInjectionShapes asserts the shared charset
// whitelist accepts legitimate plain columns and dotted JSONB paths while
// rejecting any field carrying SQL escape sequences.
func TestValidateFilterField_RejectsInjectionShapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   string
		wantErr bool
	}{
		{name: "plain column", field: "status", wantErr: false},
		{name: "dotted jsonb path", field: "metadata.foo", wantErr: false},
		{name: "deep dotted path", field: "fee_charge.totalAmount", wantErr: false},
		{name: "leading underscore root", field: "_internal.flag", wantErr: false},
		{name: "sql injection via dotted path", field: "metadata.x) OR (1=1) --", wantErr: true},
		{name: "whitespace in field", field: "status OR 1=1", wantErr: true},
		{name: "leading digit root", field: "1status", wantErr: true},
		{name: "empty field", field: "", wantErr: true},
		{name: "trailing dot", field: "metadata.", wantErr: true},
		{name: "quote breakout", field: `id";DROP TABLE x`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateFilterField(tc.field)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestApplyPostgresFilters_RejectsInjectionFieldAtEngineGate proves the worker
// engine gate (applyPostgresFilters, feeding the squirrel Eq/Gt/... sinks)
// rejects an injection-shaped field BEFORE it can reach the verbatim,
// unquoted map key — even though its dotted root ("metadata") is a real column.
func TestApplyPostgresFilters_RejectsInjectionFieldAtEngineGate(t *testing.T) {
	t.Parallel()

	schema := buildSnapshot("ledger", map[string][]string{
		"public.accounts": {"id", "status", "metadata"},
	})

	builder := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).
		Select("*").From(`"public"."accounts"`)

	injectionField := "metadata.x) OR (1=1) --"
	filters := map[string]model.FilterCondition{
		injectionField: {Equals: []any{"x"}},
	}

	_, err := applyPostgresFilters(builder, "public.accounts", filters, schema)
	require.Error(t, err, "injection-shaped field must be rejected at the engine gate")
	assert.Contains(t, err.Error(), "invalid filter field name")

	// Belt-and-suspenders: the malformed field must never reach the SQL string.
	gotBuilder, _ := applyPostgresFilters(builder, "public.accounts", map[string]model.FilterCondition{}, schema)
	sql, _, sqlErr := gotBuilder.ToSql()
	require.NoError(t, sqlErr)
	assert.False(t, strings.Contains(sql, "OR (1=1)"), "injection payload must never reach assembled SQL")
}

// TestApplyPostgresFilters_AcceptsValidDottedAndPlainFields proves a valid dotted
// JSONB path and a plain column both pass the gate and produce WHERE clauses.
func TestApplyPostgresFilters_AcceptsValidDottedAndPlainFields(t *testing.T) {
	t.Parallel()

	schema := buildSnapshot("ledger", map[string][]string{
		"public.accounts": {"id", "status", "metadata"},
	})

	builder := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar).
		Select("*").From(`"public"."accounts"`)

	filters := map[string]model.FilterCondition{
		"status":       {Equals: []any{"ACTIVE"}},
		"metadata.foo": {Equals: []any{"bar"}},
	}

	out, err := applyPostgresFilters(builder, "public.accounts", filters, schema)
	require.NoError(t, err, "valid plain column and dotted JSONB field must be accepted")

	sql, args, sqlErr := out.ToSql()
	require.NoError(t, sqlErr)
	assert.Contains(t, sql, "status")
	assert.Contains(t, sql, "metadata.foo")
	assert.Len(t, args, 2)
}
