// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestLimitEnumValidators drives the limittype and limitstatus custom
// validators through the shared validator instance, asserting the model-valid
// enum values pass and unknown values are rejected.
func TestLimitEnumValidators(t *testing.T) {
	v, err := getValidator()
	require.NoError(t, err)

	t.Run("limittype accepts every model-valid type", func(t *testing.T) {
		for _, lt := range []model.LimitType{
			model.LimitTypeDaily, model.LimitTypeWeekly, model.LimitTypeMonthly,
			model.LimitTypeCustom, model.LimitTypePerTransaction,
		} {
			require.True(t, lt.IsValid(), "guard: %q must be model-valid", lt)
			assert.NoError(t, v.Var(string(lt), "limittype"), "%q should pass limittype", lt)
		}
	})

	t.Run("limittype rejects unknown type", func(t *testing.T) {
		assert.Error(t, v.Var("HOURLY", "limittype"))
	})

	t.Run("limitstatus accepts every model-valid status", func(t *testing.T) {
		for _, ls := range []model.LimitStatus{
			model.LimitStatusDraft, model.LimitStatusActive,
			model.LimitStatusInactive, model.LimitStatusDeleted,
		} {
			require.True(t, ls.IsValid(), "guard: %q must be model-valid", ls)
			assert.NoError(t, v.Var(string(ls), "limitstatus"), "%q should pass limitstatus", ls)
		}
	})

	t.Run("limitstatus rejects unknown status", func(t *testing.T) {
		assert.Error(t, v.Var("SUSPENDED", "limitstatus"))
	})
}

// TestToLimitJSONFieldName pins the struct-field -> JSON-field mapping used to
// render limit validation error messages. Known fields map to their camelCase
// JSON names; an unknown field falls through unchanged.
func TestToLimitJSONFieldName(t *testing.T) {
	cases := map[string]string{
		"Name":        "name",
		"Description": "description",
		"LimitType":   "limitType",
		"MaxAmount":   "maxAmount",
		"Currency":    "currency",
		"Scopes":      "scopes",
		"Status":      "status",
		"Limit":       "limit",
		"Cursor":      "cursor",
		"SortBy":      "sortBy",
		"SortOrder":   "sortOrder",
	}

	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, toLimitJSONFieldName(in))
		})
	}

	t.Run("unknown field falls through unchanged", func(t *testing.T) {
		assert.Equal(t, "SomethingElse", toLimitJSONFieldName("SomethingElse"))
	})
}

// TestToLimitScopeJSONFieldName pins the scope struct-field -> JSON-field
// mapping used in scope validation error messages.
func TestToLimitScopeJSONFieldName(t *testing.T) {
	cases := map[string]string{
		"SegmentID":       "segmentId",
		"PortfolioID":     "portfolioId",
		"AccountID":       "accountId",
		"MerchantID":      "merchantId",
		"TransactionType": "transactionType",
		"SubType":         "subType",
	}

	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, toLimitScopeJSONFieldName(in))
		})
	}

	t.Run("unknown field falls through unchanged", func(t *testing.T) {
		assert.Equal(t, "Unknown", toLimitScopeJSONFieldName("Unknown"))
	})
}
