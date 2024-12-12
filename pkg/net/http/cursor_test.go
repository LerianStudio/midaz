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

func TestApplyCursorPagination(t *testing.T) {
	decodedCursor := CreateCursor("test_id", true)
	orderDirection := strings.ToUpper(string(constant.Asc))

	query := squirrel.Select("*").From("test_table")
	expectedQuery := query.Where(squirrel.Expr("id >= ?", "test_id")).OrderBy("id DESC")

	resultQuery := ApplyCursorPagination(query, decodedCursor, orderDirection)
	sqlResult, _, _ := resultQuery.ToSql()
	sqlExpected, _, _ := expectedQuery.ToSql()

	assert.Equal(t, sqlExpected, sqlResult)
}

func TestPaginateRecords(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}
	limit := 3

	result := PaginateRecords(true, false, true, items, limit)
	expected := []int{1, 2, 3}
	assert.Equal(t, expected, result)

	result = PaginateRecords(false, true, true, items, limit)
	expected = []int{5, 4, 3}
	assert.Equal(t, expected, result)

	result = PaginateRecords(false, true, false, items, limit)
	expected = []int{3, 4, 5}
	assert.Equal(t, expected, result)
}

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
}
