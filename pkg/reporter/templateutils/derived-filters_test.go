// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package templateutils

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func sortedFields(m map[string]map[string][]string, db, table string) []string {
	f := append([]string{}, m[db][table]...)
	sort.Strings(f)

	return f
}

// The balancete template: add_day + add_signed applied to the operation table.
// Expectation: dia/signed dropped, created_at/amount present alongside the real
// columns route_code/direction. So the worker SELECTs only real columns.
func TestRewriteDerivedFilterFields_Balancete(t *testing.T) {
	t.Parallel()

	tpl := `DATA;CONTA COSIF;DEBITO;CREDITO;SALDO;SALDO FINAL
{%- with ops = midaz_transaction:transaction_5b37095b923845b98f007e7e9f25d414.operation|add_day:"created_at"|add_signed:"amount" -%}
{% last_item_by_group ops group_by "dia,route_code" order_by "created_at" as linhas -%}
{%- for l in linhas -%}
{%- if l.route_code -%}
{{ l.created_at|date:"02/01/06" }};{{ l.route_code }};{% sum_by ops by "signed" if route_code == l.route_code and direction == "debit" and dia == l.dia %};{% sum_by ops by "amount" if route_code == l.route_code and direction == "credit" and dia == l.dia %};{% sum_by ops by "signed" if route_code == l.route_code and dia == l.dia %};{% sum_by ops by "signed" if route_code == l.route_code and dia <= l.dia %}
{% endif -%}
{%- endfor -%}
{%- endwith -%}`

	got := MappedFieldsOfTemplate(tpl)

	const db = "midaz_transaction"
	const table = "transaction_5b37095b923845b98f007e7e9f25d414__operation"

	fields := sortedFields(got, db, table)
	assert.Equal(t, []string{"amount", "created_at", "direction", "route_code"}, fields)
	assert.NotContains(t, fields, "dia")
	assert.NotContains(t, fields, "signed")
}

// Legacy (no explicit schema) reference must work too.
func TestRewriteDerivedFilterFields_LegacyRef(t *testing.T) {
	t.Parallel()

	tpl := `{%- with ops = midaz_transaction.operation|add_day:"created_at"|add_signed:"amount" -%}
{% sum_by ops by "signed" if dia == 1 %};{{ ops|sum:"signed" }}
{%- endwith -%}`

	got := MappedFieldsOfTemplate(tpl)
	fields := sortedFields(got, "midaz_transaction", "operation")

	assert.Contains(t, fields, "created_at")
	assert.Contains(t, fields, "amount")
	assert.NotContains(t, fields, "dia")
	assert.NotContains(t, fields, "signed")
}

// The `as` result variable of last_item_by_group (e.g. "linhas") must not leak
// as a phantom top-level datasource with no tables, which breaks the fetcher
// (FET-0401 - datasource must have at least one table with fields).
func TestMappedFields_NoPhantomDatasource(t *testing.T) {
	t.Parallel()

	tpl := `DATA;CONTA COSIF;DEBITO;CREDITO;SALDO;SALDO FINAL
{%- with ops = midaz_transaction.operation|add_day:"created_at"|add_signed:"amount" -%}
{% last_item_by_group ops group_by "dia,route_code" order_by "created_at" as linhas -%}
{%- for l in linhas -%}
{%- if l.route_code -%}
{{ l.created_at|date:"02/01/06" }};{{ l.route_code }};{% sum_by ops by "signed" if route_code == l.route_code and direction == "debit" and dia == l.dia %}
{% endif -%}
{%- endfor -%}
{%- endwith -%}`

	got := MappedFieldsOfTemplate(tpl)

	_, hasPhantom := got["linhas"]
	assert.False(t, hasPhantom, "last_item_by_group `as` var leaked as a datasource: %#v", got)
	assert.Contains(t, got, "midaz_transaction")
	assert.NotEmpty(t, got["midaz_transaction"]["operation"])
}

func TestPruneEmptyDatasources(t *testing.T) {
	t.Parallel()

	in := map[string]map[string][]string{
		"linhas":            {},                            // phantom -> removed
		"midaz_transaction": {"operation": {"route_code"}}, // real -> kept
		"empty_ds":          {},                            // phantom -> removed
	}

	pruneEmptyDatasources(in)

	assert.NotContains(t, in, "linhas")
	assert.NotContains(t, in, "empty_ds")
	assert.Contains(t, in, "midaz_transaction")
}

// A template using neither filter must be left untouched (and any real column
// literally named "dia" or "signed" must survive).
func TestRewriteDerivedFilterFields_NoopWithoutFilters(t *testing.T) {
	t.Parallel()

	before := map[string]map[string][]string{
		"ds": {"t": {"dia", "signed", "amount"}},
	}

	rewriteDerivedFilterFields(before, `{{ ds.t.dia }} {{ ds.t.signed }} {{ ds.t.amount }}`)

	assert.ElementsMatch(t, []string{"dia", "signed", "amount"}, before["ds"]["t"])
}
