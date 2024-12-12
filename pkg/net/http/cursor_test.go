package http

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/assert"
)

func TestDecodeCursor(t *testing.T) {
	cursor := CreateCursor("test_id", true)
	encodedCursor := base64.StdEncoding.EncodeToString([]byte(`{"id":"test_id","points_next":true}`))

	decodedCursor, err := DecodeCursor(encodedCursor)
	assert.NoError(t, err)
	assert.Equal(t, cursor, decodedCursor)
}

// TestApplyCursorPaginationDesc tests the ApplyCursorPagination function with descending order.
func TestApplyCursorPaginationDesc(t *testing.T) {
	query := squirrel.Select("*").From("test_table")
	decodedCursor := CreateCursor("test_id", true)
	orderDirection := strings.ToUpper(string(constant.Desc))
	limit := 10

	resultQuery, resultOrder := ApplyCursorPagination(query, decodedCursor, orderDirection, limit)
	sqlResult, _, _ := resultQuery.ToSql()
	expectedQuery := query.Where(squirrel.Expr("id < ?", "test_id")).OrderBy("id DESC").Limit(uint64(limit + 1))
	sqlExpected, _, _ := expectedQuery.ToSql()

	assert.Equal(t, sqlExpected, sqlResult)
	assert.Equal(t, orderDirection, resultOrder)
}

// TestApplyCursorPaginationNoCursor tests the ApplyCursorPagination function with no cursor.
func TestApplyCursorPaginationNoCursor(t *testing.T) {
	query := squirrel.Select("*").From("test_table")
	decodedCursor := CreateCursor("", true)
	orderDirection := strings.ToUpper(string(constant.Asc))
	limit := 10

	resultQuery, resultOrder := ApplyCursorPagination(query, decodedCursor, orderDirection, limit)
	sqlResult, _, _ := resultQuery.ToSql()
	expectedQuery := query.OrderBy("id ASC").Limit(uint64(limit + 1))
	sqlExpected, _, _ := expectedQuery.ToSql()

	assert.Equal(t, sqlExpected, sqlResult)
	assert.Equal(t, orderDirection, resultOrder)
}

// TestApplyCursorPaginationPrevPage tests the ApplyCursorPagination function for previous page.
func TestApplyCursorPaginationPrevPage(t *testing.T) {
	query := squirrel.Select("*").From("test_table")
	decodedCursor := CreateCursor("test_id", false)
	orderDirection := strings.ToUpper(string(constant.Asc))
	limit := 10

	resultQuery, resultOrder := ApplyCursorPagination(query, decodedCursor, orderDirection, limit)
	sqlResult, _, _ := resultQuery.ToSql()
	expectedQuery := query.Where(squirrel.Expr("id < ?", "test_id")).OrderBy("id DESC").Limit(uint64(limit + 1))
	sqlExpected, _, _ := expectedQuery.ToSql()

	assert.Equal(t, sqlExpected, sqlResult)
	assert.Equal(t, orderDirection, resultOrder)
}

// TestApplyCursorPaginationPrevPageDesc tests the ApplyCursorPagination function for previous page with descending order.
func TestApplyCursorPaginationPrevPageDesc(t *testing.T) {
	query := squirrel.Select("*").From("test_table")
	decodedCursor := CreateCursor("test_id", false)
	orderDirection := strings.ToUpper(string(constant.Desc))
	limit := 10

	resultQuery, resultOrder := ApplyCursorPagination(query, decodedCursor, orderDirection, limit)
	sqlResult, _, _ := resultQuery.ToSql()
	expectedQuery := query.Where(squirrel.Expr("id > ?", "test_id")).OrderBy("id ASC").Limit(uint64(limit + 1))
	sqlExpected, _, _ := expectedQuery.ToSql()

	assert.Equal(t, sqlExpected, sqlResult)
	assert.Equal(t, orderDirection, resultOrder)
}

// TestPaginateRecords tests the PaginateRecords function.
func TestPaginateRecords(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	limit := 3

	result := PaginateRecords(true, true, true, items, limit, "ASC")
	expected := []int{1, 2, 3}
	assert.Equal(t, expected, result)

	result = PaginateRecords(false, true, true, items, limit, "ASC")
	expected = []int{1, 2, 3}
	assert.Equal(t, expected, result)

	result = PaginateRecords(false, true, false, items, limit, "ASC")
	expected = []int{3, 2, 1}
	assert.Equal(t, expected, result)

	result = PaginateRecords(true, true, true, items, limit, "DESC")
	expected = []int{3, 2, 1}
	assert.Equal(t, expected, result)

	result = PaginateRecords(false, true, true, items, limit, "DESC")
	expected = []int{3, 2, 1}
	assert.Equal(t, expected, result)

	result = PaginateRecords(false, true, false, items, limit, "DESC")
	expected = []int{1, 2, 3}
	assert.Equal(t, expected, result)
}

// TestCalculateCursor tests the CalculateCursor function.
func TestCalculateCursor(t *testing.T) {
	firstItemID := "first_id"
	lastItemID := "last_id"

	pagination, err := CalculateCursor(true, true, true, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.NotEmpty(t, pagination.Next)
	assert.Empty(t, pagination.Prev)

	pagination, err = CalculateCursor(false, true, true, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.NotEmpty(t, pagination.Next)
	assert.NotEmpty(t, pagination.Prev)

	pagination, err = CalculateCursor(false, false, false, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.NotEmpty(t, pagination.Next)
	assert.Empty(t, pagination.Prev)

	// Additional test cases
	pagination, err = CalculateCursor(true, false, true, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.Empty(t, pagination.Next)
	assert.Empty(t, pagination.Prev)

	pagination, err = CalculateCursor(true, true, false, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.NotEmpty(t, pagination.Next)
	assert.Empty(t, pagination.Prev)

	pagination, err = CalculateCursor(false, true, false, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.NotEmpty(t, pagination.Next)
	assert.NotEmpty(t, pagination.Prev)

	pagination, err = CalculateCursor(false, false, true, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.Empty(t, pagination.Next)
	assert.NotEmpty(t, pagination.Prev)
}
