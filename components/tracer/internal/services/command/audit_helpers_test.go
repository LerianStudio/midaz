// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// auditHelpersNow is a fixed instant whose time of day sits between the
// midnight and 03:30 boundaries used below, so both reset rules are exercised.
var auditHelpersNow = time.Date(2026, time.March, 10, 1, 15, 0, 0, time.UTC)

var limitToMapKeys = []string{
	"id", "name", "description", "limitType", "maxAmount", "asset", "scopes", "status",
	"activeTimeStart", "activeTimeEnd", "resetTime", "customStartDate", "customEndDate",
	"resetAt", "createdAt", "updatedAt",
}

func mustTimeOfDay(t *testing.T, s string) *model.TimeOfDay {
	t.Helper()

	tod, err := model.NewTimeOfDay(s)
	require.NoError(t, err)

	return &tod
}

func baseAuditLimit(limitType model.LimitType) *model.Limit {
	accountID := testutil.MustDeterministicUUID(9101)
	createdAt := time.Date(2026, time.January, 5, 12, 30, 45, 123000000, time.UTC)

	return &model.Limit{
		ID:        testutil.MustDeterministicUUID(9100),
		Name:      "audit limit",
		LimitType: limitType,
		MaxAmount: decimal.RequireFromString("1500.50"),
		Asset:     "USD",
		Scopes:    []model.Scope{{AccountID: &accountID}},
		Status:    model.LimitStatusActive,
		CreatedAt: createdAt,
		UpdatedAt: createdAt.Add(time.Hour),
	}
}

func TestLimitToMap(t *testing.T) {
	t.Parallel()

	customStart := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
	customEnd := time.Date(2030, time.January, 31, 23, 59, 59, 0, time.UTC)
	description := "caps retail outflows"

	tests := []struct {
		name  string
		build func(t *testing.T) *model.Limit
		want  map[string]any
	}{
		{
			name: "daily with window, reset time and description",
			build: func(t *testing.T) *model.Limit {
				t.Helper()

				l := baseAuditLimit(model.LimitTypeDaily)
				l.Description = &description
				l.ActiveTimeStart = mustTimeOfDay(t, "09:00")
				l.ActiveTimeEnd = mustTimeOfDay(t, "17:00")
				l.ResetTime = mustTimeOfDay(t, "03:30")

				return l
			},
			want: map[string]any{
				"description":     description,
				"activeTimeStart": "09:00",
				"activeTimeEnd":   "17:00",
				"resetTime":       "03:30",
				"customStartDate": nil,
				"customEndDate":   nil,
				"resetAt":         "2026-03-10T03:30:00Z",
			},
		},
		{
			name: "daily without optional fields resets at next midnight",
			build: func(t *testing.T) *model.Limit {
				t.Helper()

				return baseAuditLimit(model.LimitTypeDaily)
			},
			want: map[string]any{
				"description":     nil,
				"activeTimeStart": nil,
				"activeTimeEnd":   nil,
				"resetTime":       nil,
				"customStartDate": nil,
				"customEndDate":   nil,
				"resetAt":         "2026-03-11T00:00:00Z",
			},
		},
		{
			name: "per transaction has no reset",
			build: func(t *testing.T) *model.Limit {
				t.Helper()

				return baseAuditLimit(model.LimitTypePerTransaction)
			},
			want: map[string]any{
				"activeTimeStart": nil,
				"activeTimeEnd":   nil,
				"resetTime":       nil,
				"customStartDate": nil,
				"customEndDate":   nil,
				"resetAt":         nil,
			},
		},
		{
			name: "custom with end date resets the day after it",
			build: func(t *testing.T) *model.Limit {
				t.Helper()

				l := baseAuditLimit(model.LimitTypeCustom)
				l.CustomStartDate = &customStart
				l.CustomEndDate = &customEnd

				return l
			},
			want: map[string]any{
				"activeTimeStart": nil,
				"activeTimeEnd":   nil,
				"resetTime":       nil,
				"customStartDate": "2030-01-01T00:00:00Z",
				"customEndDate":   "2030-01-31T23:59:59Z",
				"resetAt":         "2030-02-01T00:00:00Z",
			},
		},
		{
			name: "custom dates sent with an offset are recorded in UTC",
			build: func(t *testing.T) *model.Limit {
				t.Helper()

				brt := time.FixedZone("BRT", -3*60*60)
				start := time.Date(2030, time.January, 1, 0, 0, 0, 0, brt)
				end := time.Date(2030, time.January, 31, 23, 59, 59, 0, brt)

				l := baseAuditLimit(model.LimitTypeCustom)
				l.CustomStartDate = &start
				l.CustomEndDate = &end

				return l
			},
			want: map[string]any{
				"activeTimeStart": nil,
				"activeTimeEnd":   nil,
				"resetTime":       nil,
				"customStartDate": "2030-01-01T03:00:00Z",
				"customEndDate":   "2030-02-01T02:59:59Z",
				"resetAt":         "2030-02-02T00:00:00Z",
			},
		},
		{
			name: "custom without end date has no reset",
			build: func(t *testing.T) *model.Limit {
				t.Helper()

				l := baseAuditLimit(model.LimitTypeCustom)
				l.CustomStartDate = &customStart

				return l
			},
			want: map[string]any{
				"customStartDate": "2030-01-01T00:00:00Z",
				"customEndDate":   nil,
				"resetAt":         nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			limit := tt.build(t)

			got := LimitToMap(limit, auditHelpersNow)

			require.Len(t, got, len(limitToMapKeys))

			gotKeys := make([]string, 0, len(got))
			for k := range got {
				gotKeys = append(gotKeys, k)
			}

			require.ElementsMatch(t, limitToMapKeys, gotKeys)

			assert.Equal(t, limit.ID.String(), got["id"])
			assert.Equal(t, limit.Name, got["name"])
			assert.Equal(t, limit.LimitType, got["limitType"])
			assert.Equal(t, limit.MaxAmount, got["maxAmount"])
			assert.Equal(t, limit.Asset, got["asset"])
			assert.Equal(t, limit.Status, got["status"])
			assert.Equal(t, limit.Scopes, got["scopes"])
			assert.Equal(t, "2026-01-05T12:30:45.123Z", got["createdAt"])
			assert.Equal(t, "2026-01-05T13:30:45.123Z", got["updatedAt"])

			for key, want := range tt.want {
				assert.Equal(t, want, got[key], "key %q", key)
			}
		})
	}
}

func TestLimitToMap_IgnoresStoredResetAt(t *testing.T) {
	t.Parallel()

	limit := baseAuditLimit(model.LimitTypeDaily)
	stale := time.Date(2026, time.January, 6, 0, 0, 0, 0, time.UTC)
	limit.ResetAt = &stale

	got := LimitToMap(limit, auditHelpersNow)

	assert.Equal(t, "2026-03-11T00:00:00Z", got["resetAt"])
}

func TestLimitToMap_NilLimit(t *testing.T) {
	t.Parallel()

	assert.Nil(t, LimitToMap(nil, auditHelpersNow))
}

func TestLimitToMap_ScopesAreCopied(t *testing.T) {
	t.Parallel()

	limit := baseAuditLimit(model.LimitTypeDaily)
	original := *limit.Scopes[0].AccountID

	got := LimitToMap(limit, auditHelpersNow)

	replacement := testutil.MustDeterministicUUID(9102)
	limit.Scopes[0] = model.Scope{AccountID: &replacement}

	scopes, ok := got["scopes"].([]model.Scope)
	require.True(t, ok)
	require.Len(t, scopes, 1)
	require.NotNil(t, scopes[0].AccountID)
	assert.Equal(t, original, *scopes[0].AccountID)
}

func TestRuleToMap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		rule *model.Rule
		want map[string]any
	}{
		{
			name: "nil rule",
			rule: nil,
			want: nil,
		},
		{
			name: "timestamps use the audit layout",
			rule: &model.Rule{
				ID:        testutil.MustDeterministicUUID(9200),
				Name:      "audit rule",
				CreatedAt: time.Date(2026, time.February, 1, 8, 0, 0, 500000000, time.UTC),
				UpdatedAt: time.Date(2026, time.February, 1, 9, 0, 0, 0, time.UTC),
			},
			want: map[string]any{
				"createdAt": "2026-02-01T08:00:00.5Z",
				"updatedAt": "2026-02-01T09:00:00Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := RuleToMap(tt.rule)
			if tt.want == nil {
				assert.Nil(t, got)

				return
			}

			for key, want := range tt.want {
				assert.Equal(t, want, got[key], "key %q", key)
			}
		})
	}
}
