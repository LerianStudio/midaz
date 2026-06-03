// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"strings"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

func TestBuildScopeFilter_EmptyScopes(t *testing.T) {
	filter, args := buildScopeFilter([]model.Scope{})

	assert.Equal(t, "1=1", filter)
	assert.Empty(t, args)
}

func TestBuildScopeFilter_SingleScopeWithTransactionType(t *testing.T) {
	txType := model.TransactionTypeCard

	scopes := []model.Scope{
		{TransactionType: &txType},
	}

	filter, args := buildScopeFilter(scopes)

	assert.Contains(t, filter, "scopes = '[]'::jsonb")
	assert.Contains(t, filter, "EXISTS")
	assert.Contains(t, filter, "jsonb_array_elements")
	assert.Contains(t, filter, "transactionType")
	assert.Len(t, args, 1)
	assert.Equal(t, string(txType), args[0])
}

func TestBuildScopeFilter_SingleScopeWithMultipleFields(t *testing.T) {
	txType := model.TransactionTypePix
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	merchantID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

	scopes := []model.Scope{
		{
			TransactionType: &txType,
			AccountID:       &accountID,
			MerchantID:      &merchantID,
		},
	}

	filter, args := buildScopeFilter(scopes)

	assert.Contains(t, filter, "accountId")
	assert.Contains(t, filter, "merchantId")
	assert.Contains(t, filter, "transactionType")
	assert.Len(t, args, 3)
}

func TestBuildScopeFilter_MultipleScopes(t *testing.T) {
	txTypeCard := model.TransactionTypeCard
	txTypePix := model.TransactionTypePix

	scopes := []model.Scope{
		{TransactionType: &txTypeCard},
		{TransactionType: &txTypePix},
	}

	filter, args := buildScopeFilter(scopes)

	assert.Contains(t, filter, "OR")
	assert.Len(t, args, 2)
	assert.Equal(t, string(txTypeCard), args[0])
	assert.Equal(t, string(txTypePix), args[1])
}

func TestBuildScopeFilter_AllFields(t *testing.T) {
	txType := model.TransactionTypeCard
	segmentID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	portfolioID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440003")
	merchantID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440004")
	subType := "CREDIT"

	scopes := []model.Scope{
		{
			SegmentID:       &segmentID,
			PortfolioID:     &portfolioID,
			AccountID:       &accountID,
			MerchantID:      &merchantID,
			TransactionType: &txType,
			SubType:         &subType,
		},
	}

	filter, args := buildScopeFilter(scopes)

	assert.Contains(t, filter, "segmentId")
	assert.Contains(t, filter, "portfolioId")
	assert.Contains(t, filter, "accountId")
	assert.Contains(t, filter, "merchantId")
	assert.Contains(t, filter, "transactionType")
	assert.Contains(t, filter, "subType")
	assert.Len(t, args, 6)
}

func TestBuildSingleScopeCondition_EmptyScope(t *testing.T) {
	condition, args := buildSingleScopeCondition(model.Scope{})

	assert.Equal(t, "1=1", condition)
	assert.Empty(t, args)
}

func TestBuildSingleScopeCondition_SingleField(t *testing.T) {
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	condition, args := buildSingleScopeCondition(model.Scope{AccountID: &accountID})

	assert.Contains(t, condition, "accountId")
	assert.Contains(t, condition, "?")
	assert.Len(t, args, 1)
	assert.Equal(t, accountID.String(), args[0])
}

func TestBuildSingleScopeCondition_MultipleFields(t *testing.T) {
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	merchantID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440002")

	condition, args := buildSingleScopeCondition(model.Scope{
		AccountID:  &accountID,
		MerchantID: &merchantID,
	})

	// Should use ? placeholders, not $N
	assert.Contains(t, condition, "?")
	assert.NotContains(t, condition, "$")
	assert.Contains(t, condition, "AND")
	assert.Len(t, args, 2)
}

func TestBuildSingleScopeCondition_TransactionType(t *testing.T) {
	txType := model.TransactionTypePix

	condition, args := buildSingleScopeCondition(model.Scope{TransactionType: &txType})

	assert.Contains(t, condition, "transactionType")
	assert.Contains(t, condition, "?")
	assert.Len(t, args, 1)
	assert.Equal(t, "PIX", args[0])
}

// TestBuildSingleScopeCondition_SubType_CaseInsensitive verifies that the SQL
// condition for SubType wraps both sides in LOWER(BTRIM(...)), making the
// filter case-insensitive AND whitespace-insensitive. This keeps DB-side
// filtering symmetric with the runtime ptrMatchesFold matching used on cached
// data, so admin list queries and cache-cold paths behave identically
// regardless of SubType casing or surrounding whitespace in either the filter
// input or the persisted scope value. The filter argument itself is also
// trimmed on the Go side (strings.TrimSpace) so the arg passed to the driver
// is already canonical.
func TestBuildSingleScopeCondition_SubType_CaseInsensitive(t *testing.T) {
	subType := "  Sell  "

	condition, args := buildSingleScopeCondition(model.Scope{SubType: &subType})

	assert.Contains(t, condition, "BTRIM(scope->>'subType')",
		"condition must BTRIM the JSONB-extracted subType value so persisted whitespace does not block matches")
	assert.Contains(t, condition, "LOWER(BTRIM(scope->>'subType'))",
		"condition must lower the trimmed JSONB-extracted subType value")
	assert.Contains(t, condition, "LOWER(?)", "condition must lower the filter argument as well")
	require.Len(t, args, 1)
	require.Equal(t, "Sell", args[0],
		"the Go-side arg must be strings.TrimSpace'd before being bound; LOWER in SQL handles the case half")
}

func TestJoinAnd(t *testing.T) {
	tests := []struct {
		name       string
		conditions []string
		expected   string
	}{
		{"empty", []string{}, ""},
		{"single", []string{"a = 1"}, "a = 1"},
		{"two", []string{"a = 1", "b = 2"}, "a = 1 AND b = 2"},
		{"three", []string{"a = 1", "b = 2", "c = 3"}, "a = 1 AND b = 2 AND c = 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinAnd(tt.conditions)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJoinOr(t *testing.T) {
	tests := []struct {
		name       string
		conditions []string
		expected   string
	}{
		{"empty", []string{}, ""},
		{"single", []string{"a = 1"}, "a = 1"},
		{"two", []string{"a = 1", "b = 2"}, "a = 1 OR b = 2"},
		{"three", []string{"a = 1", "b = 2", "c = 3"}, "a = 1 OR b = 2 OR c = 3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinOr(tt.conditions)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildScopeFilter_GlobalRulesAlwaysMatch(t *testing.T) {
	txType := model.TransactionTypeCard

	scopes := []model.Scope{
		{TransactionType: &txType},
	}

	filter, _ := buildScopeFilter(scopes)

	// Must include condition for global rules (empty scopes array)
	assert.Contains(t, filter, "scopes = '[]'::jsonb")
}

func TestBuildScopeFilter_NullFieldsActAsWildcards(t *testing.T) {
	txType := model.TransactionTypeCard

	scopes := []model.Scope{
		{TransactionType: &txType},
	}

	filter, _ := buildScopeFilter(scopes)

	// The condition should check for NULL OR equal, meaning null acts as wildcard
	assert.Contains(t, filter, "IS NULL OR")
}

// TestBuildScopeFilter_PlaceholderNumbering verifies that scope filter placeholders
// are correctly numbered when combined with other WHERE clauses.
// This prevents a critical bug where hardcoded $1, $2 placeholders conflict
// with Squirrel's automatic placeholder numbering.
func TestBuildScopeFilter_PlaceholderNumbering(t *testing.T) {
	txType := model.TransactionTypeCard

	scopes := []model.Scope{
		{TransactionType: &txType},
	}

	filter, args := buildScopeFilter(scopes)

	// Simulate what ListActiveByScopes does: combine scope filter with other WHERE clauses
	query := sq.Select("*").
		From("rules").
		Where(sq.Eq{"status": "ACTIVE"}).
		Where(sq.Eq{"deleted_at": nil}).
		Where(filter, args...).
		PlaceholderFormat(sq.Dollar)

	sql, sqlArgs, err := query.ToSql()
	require.NoError(t, err)

	// Must have 2 arguments: ACTIVE and CARD
	assert.Len(t, sqlArgs, 2, "should have 2 arguments")
	assert.Equal(t, "ACTIVE", sqlArgs[0], "first arg should be status")
	assert.Equal(t, "CARD", sqlArgs[1], "second arg should be transactionType")

	// $1 must appear exactly ONCE (for status)
	// If $1 appears more than once, it means we have placeholder conflict
	dollarOneCount := strings.Count(sql, "$1")
	assert.Equal(t, 1, dollarOneCount, "placeholder $1 should appear exactly once, got %d times in SQL: %s", dollarOneCount, sql)

	// $2 must exist for the scope filter transactionType
	assert.Contains(t, sql, "$2", "placeholder $2 should exist for scope filter, SQL: %s", sql)
}

// TestBuildScopeFilter_PlaceholderNumbering_MultipleScopes verifies placeholder
// numbering with multiple scopes and fields.
func TestBuildScopeFilter_PlaceholderNumbering_MultipleScopes(t *testing.T) {
	txTypeCard := model.TransactionTypeCard
	txTypePix := model.TransactionTypePix
	accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	scopes := []model.Scope{
		{TransactionType: &txTypeCard},
		{TransactionType: &txTypePix, AccountID: &accountID},
	}

	filter, args := buildScopeFilter(scopes)

	// Simulate ListActiveByScopes
	query := sq.Select("*").
		From("rules").
		Where(sq.Eq{"status": "ACTIVE"}).
		Where(sq.Eq{"deleted_at": nil}).
		Where(filter, args...).
		PlaceholderFormat(sq.Dollar)

	sql, sqlArgs, err := query.ToSql()
	require.NoError(t, err)

	// Must have 4 arguments: ACTIVE, CARD, acc UUID string, PIX
	// Note: AccountID comes before TransactionType due to field order in buildSingleScopeCondition
	assert.Len(t, sqlArgs, 4, "should have 4 arguments")
	assert.Equal(t, "ACTIVE", sqlArgs[0])
	assert.Equal(t, "CARD", sqlArgs[1])
	assert.Equal(t, accountID.String(), sqlArgs[2]) // AccountID is processed before TransactionType
	assert.Equal(t, "PIX", sqlArgs[3])

	// Each placeholder should appear exactly once
	for i := 1; i <= 4; i++ {
		placeholder := "$" + string(rune('0'+i))
		count := strings.Count(sql, placeholder)
		assert.Equal(t, 1, count, "placeholder %s should appear exactly once, got %d times", placeholder, count)
	}
}

func TestApplyFilters_WithScopeFilter(t *testing.T) {
	repo := &Repository{}

	t.Run("no scope filter - no scope clause added", func(t *testing.T) {
		filter := &model.ListRulesFilter{
			Limit: 10,
		}

		baseQuery := sq.Select("id").From("rules").PlaceholderFormat(sq.Dollar)
		query := repo.applyFilters(baseQuery, filter)

		sqlStr, args, err := query.ToSql()
		require.NoError(t, err)

		assert.NotContains(t, sqlStr, "jsonb_array_elements")
		assert.Empty(t, args)
	})

	t.Run("with accountId scope filter - adds JSONB clause", func(t *testing.T) {
		accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		filter := &model.ListRulesFilter{
			Limit: 10,
			ScopeFilter: &model.Scope{
				AccountID: &accountID,
			},
		}

		baseQuery := sq.Select("id").From("rules").PlaceholderFormat(sq.Dollar)
		query := repo.applyFilters(baseQuery, filter)

		sqlStr, args, err := query.ToSql()
		require.NoError(t, err)

		assert.Contains(t, sqlStr, "jsonb_array_elements")
		assert.Contains(t, sqlStr, "scopes")
		assert.Contains(t, sqlStr, "accountId")
		assert.NotEmpty(t, args)
	})

	t.Run("with multiple scope fields - adds compound JSONB clause", func(t *testing.T) {
		accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		txType := model.TransactionTypeCard
		filter := &model.ListRulesFilter{
			Limit: 10,
			ScopeFilter: &model.Scope{
				AccountID:       &accountID,
				TransactionType: &txType,
			},
		}

		baseQuery := sq.Select("id").From("rules").PlaceholderFormat(sq.Dollar)
		query := repo.applyFilters(baseQuery, filter)

		sqlStr, args, err := query.ToSql()
		require.NoError(t, err)

		assert.Contains(t, sqlStr, "accountId")
		assert.Contains(t, sqlStr, "transactionType")
		assert.Len(t, args, 2)
	})

	t.Run("scope filter combined with name and status filters", func(t *testing.T) {
		name := "test"
		status := model.RuleStatusActive
		accountID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		filter := &model.ListRulesFilter{
			Name:   &name,
			Status: &status,
			Limit:  10,
			ScopeFilter: &model.Scope{
				AccountID: &accountID,
			},
		}

		baseQuery := sq.Select("id").From("rules").PlaceholderFormat(sq.Dollar)
		query := repo.applyFilters(baseQuery, filter)

		sqlStr, args, err := query.ToSql()
		require.NoError(t, err)

		assert.Contains(t, sqlStr, "ILIKE")
		assert.Contains(t, sqlStr, "jsonb_array_elements")
		assert.Contains(t, sqlStr, "status")
		assert.NotEmpty(t, args)
	})
}
