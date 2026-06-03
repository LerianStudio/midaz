// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"testing"
	"time"

	"github.com/flosch/pongo2/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLastItemByGroup_BasicGrouping(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" as results %}{% for item in results %}{{ item.account_id }}:{{ item.available_balance_after }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"account_id": "acc-1", "available_balance_after": "1000.50", "created_at": "2026-01-15T10:00:00Z"},
			{"account_id": "acc-2", "available_balance_after": "2000.00", "created_at": "2026-01-20T10:00:00Z"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "acc-1:1000.50;")
	assert.Contains(t, out, "acc-2:2000.00;")
}

func TestLastItemByGroup_MultipleItemsSameGroup(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" as results %}{% for item in results %}{{ item.account_id }}:{{ item.available_balance_after }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"account_id": "acc-1", "available_balance_after": "1000.50", "created_at": "2026-01-15T10:00:00Z"},
			{"account_id": "acc-1", "available_balance_after": "1500.75", "created_at": "2026-01-31T15:00:00Z"},
			{"account_id": "acc-1", "available_balance_after": "1200.00", "created_at": "2026-01-20T10:00:00Z"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	// Only the latest item (1500.75, dated Jan 31) should be kept
	assert.Contains(t, out, "acc-1:1500.75;")
	assert.NotContains(t, out, "1000.50")
	assert.NotContains(t, out, "1200.00")
}

func TestLastItemByGroup_WithFilter(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" if route as results %}{% for item in results %}{{ item.account_id }}:{{ item.available_balance_after }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"account_id": "acc-1", "route": "route-1", "available_balance_after": "1000.50", "created_at": "2026-01-15T10:00:00Z"},
			{"account_id": "acc-2", "route": nil, "available_balance_after": "2000.00", "created_at": "2026-01-20T10:00:00Z"},
			{"account_id": "acc-3", "route": "", "available_balance_after": "3000.00", "created_at": "2026-01-25T10:00:00Z"},
			{"account_id": "acc-4", "route": "route-2", "available_balance_after": "4000.00", "created_at": "2026-01-28T10:00:00Z"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	// Only items with truthy route should be included
	assert.Contains(t, out, "acc-1:1000.50;")
	assert.Contains(t, out, "acc-4:4000.00;")
	assert.NotContains(t, out, "acc-2")
	assert.NotContains(t, out, "acc-3")
}

func TestLastItemByGroup_WithFilterExpression(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" if type == "CREDIT" as results %}{% for item in results %}{{ item.account_id }}:{{ item.available_balance_after }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"account_id": "acc-1", "type": "CREDIT", "available_balance_after": "1000.50", "created_at": "2026-01-15T10:00:00Z"},
			{"account_id": "acc-2", "type": "DEBIT", "available_balance_after": "2000.00", "created_at": "2026-01-20T10:00:00Z"},
			{"account_id": "acc-3", "type": "CREDIT", "available_balance_after": "3000.00", "created_at": "2026-01-25T10:00:00Z"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "acc-1:1000.50;")
	assert.Contains(t, out, "acc-3:3000.00;")
	assert.NotContains(t, out, "acc-2")
}

func TestLastItemByGroup_EmptyCollection(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" as results %}count:{{ results|length }}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Equal(t, "count:0", out)
}

func TestLastItemByGroup_SyntaxError_MissingGroupBy(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data "account_id" order_by "created_at" as results %}`

	_, err := SafeFromString(tplStr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "group_by")
}

func TestLastItemByGroup_SyntaxError_MissingOrderBy(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" "created_at" as results %}`

	_, err := SafeFromString(tplStr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "order_by")
}

func TestLastItemByGroup_SyntaxError_MissingAs(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" results %}`

	_, err := SafeFromString(tplStr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "as")
}

func TestLastItemByGroup_SyntaxError_MissingVarName(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" as %}`

	_, err := SafeFromString(tplStr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "variable name")
}

func TestLastItemByGroup_RFC3339Date(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" as results %}{% for item in results %}{{ item.account_id }}:{{ item.balance }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"account_id": "acc-1", "balance": "1000.00", "created_at": "2026-01-15T10:00:00Z"},
			{"account_id": "acc-1", "balance": "2000.00", "created_at": "2026-01-31T15:30:00Z"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	// Last by RFC3339 date is 2000.00
	assert.Contains(t, out, "acc-1:2000.00;")
}

func TestLastItemByGroup_TimeTypeDate(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" as results %}{% for item in results %}{{ item.account_id }}:{{ item.balance }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"account_id": "acc-1", "balance": "1000.00", "created_at": time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
			{"account_id": "acc-1", "balance": "2000.00", "created_at": time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, out, "acc-1:2000.00;")
}

func TestLastItemByGroup_SameTimestamp_DeterministicBehavior(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" as results %}{% for item in results %}{{ item.account_id }}:{{ item.balance }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"account_id": "acc-1", "balance": "1000.00", "created_at": "2026-01-31T10:00:00Z"},
			{"account_id": "acc-1", "balance": "2000.00", "created_at": "2026-01-31T10:00:00Z"},
		},
	}

	// Run multiple times to verify deterministic behavior
	for i := 0; i < 5; i++ {
		out, err := tpl.Execute(ctx)
		require.NoError(t, err)
		// With same timestamp, later item (balance=2000) should win consistently
		assert.Contains(t, out, "acc-1:2000.00;", "iteration %d should be deterministic", i)
	}
}

func TestLastItemByGroup_InvalidCollectionType(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" as results %}done`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": "not-a-slice",
	}

	_, err = tpl.Execute(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "[]map[string]any")
}

func TestLastItemByGroup_MissingGroupField(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" as results %}{% for item in results %}{{ item.account_id }}:{{ item.balance }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"account_id": "acc-1", "balance": "1000.00", "created_at": "2026-01-15T10:00:00Z"},
			{"balance": "2000.00", "created_at": "2026-01-20T10:00:00Z"}, // missing account_id
			{"account_id": "acc-2", "balance": "3000.00", "created_at": "2026-01-25T10:00:00Z"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	// Items missing the group_by field should be skipped
	assert.Contains(t, out, "acc-1:1000.00;")
	assert.Contains(t, out, "acc-2:3000.00;")
	assert.NotContains(t, out, "2000.00")
}

func TestLastItemByGroup_PreservesAllFields(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id" order_by "created_at" as results %}{% for item in results %}{{ item.account_id }}|{{ item.route }}|{{ item.type }}|{{ item.balance }}|{{ item.status }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{
				"account_id": "acc-1",
				"route":      "route-1",
				"type":       "CREDIT",
				"balance":    "1500.75",
				"status":     "COMPLETED",
				"created_at": "2026-01-31T15:00:00Z",
			},
			{
				"account_id": "acc-1",
				"route":      "route-1",
				"type":       "DEBIT",
				"balance":    "1000.00",
				"status":     "PENDING",
				"created_at": "2026-01-15T10:00:00Z",
			},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	// The latest item (Jan 31) should be returned with ALL its original fields
	assert.Contains(t, out, "acc-1|route-1|CREDIT|1500.75|COMPLETED;")
}

func TestLastItemByGroup_WithSumBy(t *testing.T) {
	t.Parallel()
	// Simulate the real template use case
	tplStr := `{% last_item_by_group operations group_by "account_id" order_by "created_at" if route as lastOps %}{% for r in routes %}{% if r.code %}{{ r.code }}:{% sum_by lastOps by "available_balance_after" if r.id == route %};{% endif %}{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"operations": []map[string]any{
			{"account_id": "acc-1", "route": "route-1", "available_balance_after": "1000.50", "created_at": "2026-01-15T10:00:00Z"},
			{"account_id": "acc-1", "route": "route-1", "available_balance_after": "1500.75", "created_at": "2026-01-31T15:00:00Z"},
			{"account_id": "acc-2", "route": "route-1", "available_balance_after": "2000.00", "created_at": "2026-01-20T10:00:00Z"},
			{"account_id": "acc-3", "route": "route-2", "available_balance_after": "500.25", "created_at": "2026-01-25T10:00:00Z"},
			{"account_id": "acc-4", "route": nil, "available_balance_after": "9999.00", "created_at": "2026-01-28T10:00:00Z"}, // filtered out by "if route"
		},
		"routes": []map[string]any{
			{"id": "route-1", "code": "001"},
			{"id": "route-2", "code": "002"},
			{"id": "route-3", "code": "003"}, // no matching operations
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	// route-1: acc-1 last(1500.75) + acc-2 last(2000.00) = 3500.75
	assert.Contains(t, out, "001:3500.75;")
	// route-2: acc-3 last(500.25) = 500.25
	assert.Contains(t, out, "002:500.25;")
	// route-3: no matching operations = 0
	assert.Contains(t, out, "003:0;")
}

func TestLastItemByGroup_CompositeGroupBy(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id,route" order_by "created_at" as results %}{% for item in results %}{{ item.account_id }}:{{ item.route }}:{{ item.balance }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"account_id": "acc-1", "route": "route-1", "balance": "1000.00", "created_at": "2026-01-15T10:00:00Z"},
			{"account_id": "acc-1", "route": "route-1", "balance": "1500.00", "created_at": "2026-01-31T10:00:00Z"},
			{"account_id": "acc-1", "route": "route-2", "balance": "2000.00", "created_at": "2026-01-20T10:00:00Z"},
			{"account_id": "acc-2", "route": "route-1", "balance": "3000.00", "created_at": "2026-01-25T10:00:00Z"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	// acc-1/route-1: latest is 1500.00 (Jan 31)
	assert.Contains(t, out, "acc-1:route-1:1500.00;")
	// acc-1/route-2: only item is 2000.00
	assert.Contains(t, out, "acc-1:route-2:2000.00;")
	// acc-2/route-1: only item is 3000.00
	assert.Contains(t, out, "acc-2:route-1:3000.00;")
	// Should NOT contain the older acc-1/route-1 item
	assert.NotContains(t, out, "acc-1:route-1:1000.00;")
}

func TestLastItemByGroup_CompositeGroupByWithFilter(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group data group_by "account_id,route" order_by "created_at" if route as results %}{% for item in results %}{{ item.account_id }}:{{ item.route }}:{{ item.balance }};{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"data": []map[string]any{
			{"account_id": "acc-1", "route": "route-1", "balance": "1000.00", "created_at": "2026-01-15T10:00:00Z"},
			{"account_id": "acc-1", "route": nil, "balance": "5000.00", "created_at": "2026-01-31T10:00:00Z"},
			{"account_id": "acc-2", "route": "route-1", "balance": "2000.00", "created_at": "2026-01-20T10:00:00Z"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	// acc-1 with nil route should be filtered out
	assert.Contains(t, out, "acc-1:route-1:1000.00;")
	assert.Contains(t, out, "acc-2:route-1:2000.00;")
	assert.NotContains(t, out, "5000.00")
}

func TestLastItemByGroup_CompositeGroupByWithSumBy(t *testing.T) {
	t.Parallel()
	tplStr := `{% last_item_by_group operations group_by "account_id,route" order_by "created_at" if route as lastOps %}{% for r in routes %}{% if r.code %}{{ r.code }}:{% sum_by lastOps by "available_balance_after" if r.id == route %};{% endif %}{% endfor %}`

	tpl, err := SafeFromString(tplStr)
	require.NoError(t, err)

	ctx := pongo2.Context{
		"operations": []map[string]any{
			{"account_id": "acc-1", "route": "route-1", "available_balance_after": "1000.50", "created_at": "2026-01-15T10:00:00Z"},
			{"account_id": "acc-1", "route": "route-1", "available_balance_after": "1500.75", "created_at": "2026-01-31T15:00:00Z"},
			{"account_id": "acc-1", "route": "route-2", "available_balance_after": "800.00", "created_at": "2026-01-20T10:00:00Z"},
			{"account_id": "acc-2", "route": "route-1", "available_balance_after": "2000.00", "created_at": "2026-01-20T10:00:00Z"},
			{"account_id": "acc-3", "route": "route-2", "available_balance_after": "500.25", "created_at": "2026-01-25T10:00:00Z"},
		},
		"routes": []map[string]any{
			{"id": "route-1", "code": "001"},
			{"id": "route-2", "code": "002"},
		},
	}

	out, err := tpl.Execute(ctx)
	require.NoError(t, err)
	// route-1: acc-1 last(1500.75) + acc-2 last(2000.00) = 3500.75
	assert.Contains(t, out, "001:3500.75;")
	// route-2: acc-1 last(800.00) + acc-3 last(500.25) = 1300.25
	assert.Contains(t, out, "002:1300.25;")
}
