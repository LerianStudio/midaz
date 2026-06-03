// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"strings"
	"testing"
	"time"

	"github.com/flosch/pongo2/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddDayFilter_StringInput(t *testing.T) {
	t.Parallel()

	in := []map[string]any{
		{"created_at": "2026-05-01T09:00:00Z"},
		{"created_at": "2026-05-31T23:59:59.999Z"},
	}

	out, perr := addDayFilter(pongo2.AsValue(in), pongo2.AsValue("created_at"))
	require.Nil(t, perr)

	got, ok := out.Interface().([]map[string]any)
	require.True(t, ok)
	assert.Equal(t, 20260501, got[0]["dia"])
	assert.Equal(t, 20260531, got[1]["dia"])
}

func TestAddDayFilter_TimeInput(t *testing.T) {
	t.Parallel()

	ts, _ := time.Parse(time.RFC3339, "2026-05-02T10:00:00Z")
	in := []map[string]any{{"created_at": ts}}

	out, perr := addDayFilter(pongo2.AsValue(in), pongo2.AsValue("created_at"))
	require.Nil(t, perr)

	got, _ := out.Interface().([]map[string]any)
	assert.Equal(t, 20260502, got[0]["dia"])
}

func TestAddDayFilter_MissingOrUnparseable(t *testing.T) {
	t.Parallel()

	in := []map[string]any{
		{"other": "x"},               // missing created_at
		{"created_at": "not-a-date"}, // unparseable
	}

	out, perr := addDayFilter(pongo2.AsValue(in), pongo2.AsValue("created_at"))
	require.Nil(t, perr)

	got, _ := out.Interface().([]map[string]any)
	_, has0 := got[0]["dia"]
	_, has1 := got[1]["dia"]
	assert.False(t, has0)
	assert.False(t, has1)
}

func TestAddDayFilter_Errors(t *testing.T) {
	t.Parallel()

	_, perr := addDayFilter(pongo2.AsValue("not a slice"), pongo2.AsValue("created_at"))
	assert.NotNil(t, perr)

	_, perr = addDayFilter(pongo2.AsValue([]map[string]any{}), pongo2.AsValue(""))
	assert.NotNil(t, perr)
}

func TestAddSignedFilter_DebitNegativeCreditPositive(t *testing.T) {
	t.Parallel()

	in := []map[string]any{
		{"amount": "100", "direction": "debit"},
		{"amount": "200", "direction": "credit"},
		{"amount": "50.2", "direction": "DEBIT"}, // case-insensitive
		{"amount": "10", "direction": ""},        // no direction -> positive
	}

	out, perr := addSignedFilter(pongo2.AsValue(in), pongo2.AsValue("amount"))
	require.Nil(t, perr)

	got, _ := out.Interface().([]map[string]any)
	assert.Equal(t, "-100", got[0]["signed"])
	assert.Equal(t, "200", got[1]["signed"])
	assert.Equal(t, "-50.2", got[2]["signed"])
	assert.Equal(t, "10", got[3]["signed"])
}

func TestAddSignedFilter_Errors(t *testing.T) {
	t.Parallel()

	_, perr := addSignedFilter(pongo2.AsValue(42), pongo2.AsValue("amount"))
	assert.NotNil(t, perr)

	_, perr = addSignedFilter(pongo2.AsValue([]map[string]any{}), pongo2.AsValue(""))
	assert.NotNil(t, perr)
}

// TestBalancete_RenderFull validates the full monthly balancete report end-to-end:
// add_day + add_signed feeding last_item_by_group group_by "dia,route_code" and
// the six columns DATA;CONTA COSIF;DEBITO;CREDITO;SALDO;SALDO FINAL (running balance).
func TestBalancete_RenderFull(t *testing.T) {
	t.Parallel()

	mk := func(code, dir, amt, ts string) map[string]any {
		parsed, _ := time.Parse(time.RFC3339, ts)
		return map[string]any{"route_code": code, "direction": dir, "amount": amt, "created_at": parsed}
	}
	ops := []map[string]any{
		mk("4.1.9.30.10.01.00005", "debit", "100", "2026-05-01T09:00:00Z"),
		mk("4.1.9.30.10.01.00005", "credit", "200", "2026-05-01T18:00:00Z"),
		mk("4.0.0.00.00.00.00001", "debit", "200", "2026-05-01T10:00:00Z"),
		mk("4.0.0.00.00.00.00001", "credit", "30", "2026-05-01T11:00:00Z"),
		mk("4.1.9.30.10.01.00005", "debit", "98", "2026-05-02T09:00:00Z"),
		mk("4.1.9.30.10.01.00005", "credit", "100", "2026-05-02T18:00:00Z"),
		mk("4.0.0.00.00.00.00001", "debit", "90", "2026-05-02T10:00:00Z"),
		mk("4.0.0.00.00.00.00001", "credit", "100", "2026-05-02T11:00:00Z"),
	}

	tpl := strings.Join([]string{
		`{% with ops = data|add_day:"created_at"|add_signed:"amount" %}`,
		`DATA;CONTA COSIF;DEBITO;CREDITO;SALDO;SALDO FINAL`, "\n",
		`{% last_item_by_group ops group_by "dia,route_code" order_by "created_at" as linhas %}`,
		`{% for l in linhas %}`,
		`{{ l.created_at|date:"02/01/06" }};{{ l.route_code }};`,
		`{% sum_by ops by "signed" if route_code == l.route_code and direction == "debit" and dia == l.dia %};`,
		`{% sum_by ops by "amount" if route_code == l.route_code and direction == "credit" and dia == l.dia %};`,
		`{% sum_by ops by "signed" if route_code == l.route_code and dia == l.dia %};`,
		`{% sum_by ops by "signed" if route_code == l.route_code and dia <= l.dia %}`, "\n",
		`{% endfor %}{% endwith %}`,
	}, "")

	parsed, err := SafeFromString(tpl)
	require.NoError(t, err)

	out, err := parsed.Execute(pongo2.Context{"data": ops})
	require.NoError(t, err)

	// DATA;COSIF;DEBITO;CREDITO;SALDO;SALDO FINAL
	assert.Contains(t, out, "01/05/26;4.0.0.00.00.00.00001;-200;30;-170;-170")
	assert.Contains(t, out, "01/05/26;4.1.9.30.10.01.00005;-100;200;100;100")
	assert.Contains(t, out, "02/05/26;4.0.0.00.00.00.00001;-90;100;10;-160") // SF acumulado: -170+10
	assert.Contains(t, out, "02/05/26;4.1.9.30.10.01.00005;-98;100;2;102")   // SF acumulado: 100+2
}
