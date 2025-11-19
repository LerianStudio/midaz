package http

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeCursor(t *testing.T) {
	cursor := CreateCursor("test_id", true)
	encodedCursor := base64.StdEncoding.EncodeToString([]byte(`{"id":"test_id","points_next":true}`))

	decodedCursor, err := DecodeCursor(encodedCursor)
	assert.NoError(t, err)
	assert.Equal(t, cursor, decodedCursor)
}

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
	assert.Equal(t, "DESC", resultOrder)
}

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
	assert.Equal(t, "ASC", resultOrder)
}

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
	assert.Equal(t, "DESC", resultOrder)
}

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
	assert.Equal(t, "ASC", resultOrder)
}

func TestPaginateRecords(t *testing.T) {
	limit := 3

	items1 := []int{1, 2, 3, 4, 5}
	result := PaginateRecords(true, true, true, items1, limit, "ASC")
	assert.Equal(t, []int{1, 2, 3}, result)

	items2 := []int{1, 2, 3, 4, 5}
	result = PaginateRecords(false, true, true, items2, limit, "ASC")
	assert.Equal(t, []int{1, 2, 3}, result)

	items3 := []int{1, 2, 3, 4, 5}
	result = PaginateRecords(false, true, false, items3, limit, "ASC")
	assert.Equal(t, []int{3, 2, 1}, result)

	items4 := []int{1, 2, 3, 4, 5}
	result = PaginateRecords(true, true, true, items4, limit, "DESC")
	assert.Equal(t, []int{1, 2, 3}, result)

	items5 := []int{1, 2, 3, 4, 5}
	result = PaginateRecords(false, true, true, items5, limit, "DESC")
	assert.Equal(t, []int{1, 2, 3}, result)

	items6 := []int{1, 2, 3, 4, 5}
	result = PaginateRecords(false, true, false, items6, limit, "DESC")
	assert.Equal(t, []int{3, 2, 1}, result)
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

	pagination, err = CalculateCursor(false, true, false, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.NotEmpty(t, pagination.Next)
	assert.NotEmpty(t, pagination.Prev)

	pagination, err = CalculateCursor(true, false, true, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.Empty(t, pagination.Next)
	assert.Empty(t, pagination.Prev)

	pagination, err = CalculateCursor(false, false, true, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.Empty(t, pagination.Next)
	assert.NotEmpty(t, pagination.Prev)

	pagination, err = CalculateCursor(false, false, false, firstItemID, lastItemID)
	assert.NoError(t, err)
	assert.Empty(t, pagination.Next)
	assert.NotEmpty(t, pagination.Prev)
}

func TestCursorWithUUIDv7(t *testing.T) {
	uuid2, err := uuid.NewV7()
	require.NoError(t, err)

	cursor := CreateCursor(uuid2.String(), true)
	cursorBytes, err := json.Marshal(cursor)
	require.NoError(t, err)
	encodedCursor := base64.StdEncoding.EncodeToString(cursorBytes)

	decodedCursor, err := DecodeCursor(encodedCursor)
	assert.NoError(t, err)
	assert.Equal(t, uuid2.String(), decodedCursor.ID)
	assert.True(t, decodedCursor.PointsNext)
}

func TestApplyCursorPaginationWithUUIDv7(t *testing.T) {
	uuid2, err := uuid.NewV7()
	require.NoError(t, err)

	tests := []struct {
		name           string
		cursorID       string
		pointsNext     bool
		orderDirection string
		expectedOp     string
		expectedOrder  string
	}{
		{
			name:           "next page with UUID v7 - ASC",
			cursorID:       uuid2.String(),
			pointsNext:     true,
			orderDirection: "ASC",
			expectedOp:     ">",
			expectedOrder:  "ASC",
		},
		{
			name:           "next page with UUID v7 - DESC",
			cursorID:       uuid2.String(),
			pointsNext:     true,
			orderDirection: "DESC",
			expectedOp:     "<",
			expectedOrder:  "DESC",
		},
		{
			name:           "prev page with UUID v7 - ASC",
			cursorID:       uuid2.String(),
			pointsNext:     false,
			orderDirection: "ASC",
			expectedOp:     "<",
			expectedOrder:  "DESC",
		},
		{
			name:           "prev page with UUID v7 - DESC",
			cursorID:       uuid2.String(),
			pointsNext:     false,
			orderDirection: "DESC",
			expectedOp:     ">",
			expectedOrder:  "ASC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := squirrel.Select("*").From("test_table")
			decodedCursor := CreateCursor(tt.cursorID, tt.pointsNext)
			limit := 10

			resultQuery, resultOrder := ApplyCursorPagination(query, decodedCursor, tt.orderDirection, limit)
			sqlResult, args, err := resultQuery.ToSql()
			require.NoError(t, err)

			expectedQuery := query.Where(squirrel.Expr("id "+tt.expectedOp+" ?", tt.cursorID)).
				OrderBy("id " + tt.expectedOrder).
				Limit(uint64(limit + 1))
			sqlExpected, expectedArgs, err := expectedQuery.ToSql()
			require.NoError(t, err)

			assert.Equal(t, sqlExpected, sqlResult)
			assert.Equal(t, expectedArgs, args)
			assert.Equal(t, tt.expectedOrder, resultOrder)
		})
	}
}

func TestPaginateRecordsWithUUIDv7(t *testing.T) {
	uuids := make([]uuid.UUID, 5)
	for i := 0; i < 5; i++ {
		var err error
		uuids[i], err = uuid.NewV7()
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond)
	}

	items := make([]string, len(uuids))
	for i, u := range uuids {
		items[i] = u.String()
	}

	limit := 3

	result1 := PaginateRecords(true, true, true, append([]string{}, items...), limit, "ASC")
	assert.Equal(t, items[:3], result1)

	result2 := PaginateRecords(false, true, false, append([]string{}, items...), limit, "ASC")
	expected := []string{items[2], items[1], items[0]}
	assert.Equal(t, expected, result2)
}

func TestCalculateCursorWithUUIDv7(t *testing.T) {
	firstUUID, err := uuid.NewV7()
	require.NoError(t, err)
	time.Sleep(1 * time.Millisecond)
	lastUUID, err := uuid.NewV7()
	require.NoError(t, err)

	firstItemID := firstUUID.String()
	lastItemID := lastUUID.String()

	tests := []struct {
		name          string
		isFirstPage   bool
		hasPagination bool
		pointsNext    bool
		expectNext    bool
		expectPrev    bool
	}{
		{
			name:          "first page with pagination - points next",
			isFirstPage:   true,
			hasPagination: true,
			pointsNext:    true,
			expectNext:    true,
			expectPrev:    false,
		},
		{
			name:          "middle page with pagination - points next",
			isFirstPage:   false,
			hasPagination: true,
			pointsNext:    true,
			expectNext:    true,
			expectPrev:    true,
		},
		{
			name:          "page with pagination - points prev",
			isFirstPage:   false,
			hasPagination: true,
			pointsNext:    false,
			expectNext:    true,
			expectPrev:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pagination, err := CalculateCursor(tt.isFirstPage, tt.hasPagination, tt.pointsNext, firstItemID, lastItemID)
			require.NoError(t, err)

			if tt.expectNext {
				assert.NotEmpty(t, pagination.Next)

				decodedNext, err := DecodeCursor(pagination.Next)
				require.NoError(t, err)
				assert.Equal(t, lastItemID, decodedNext.ID)
				assert.True(t, decodedNext.PointsNext)
			} else {
				assert.Empty(t, pagination.Next)
			}

			if tt.expectPrev {
				assert.NotEmpty(t, pagination.Prev)

				decodedPrev, err := DecodeCursor(pagination.Prev)
				require.NoError(t, err)
				assert.Equal(t, firstItemID, decodedPrev.ID)
				assert.False(t, decodedPrev.PointsNext)
			} else {
				assert.Empty(t, pagination.Prev)
			}
		})
	}
}

func TestUUIDv7TimestampOrdering(t *testing.T) {
	uuids := make([]uuid.UUID, 10)
	timestamps := make([]time.Time, 10)

	for i := 0; i < 10; i++ {
		timestamps[i] = time.Now()
		var err error
		uuids[i], err = uuid.NewV7()
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond)
	}

	for i := 0; i < 9; i++ {
		uuid1Str := uuids[i].String()
		uuid2Str := uuids[i+1].String()

		assert.True(t, uuid1Str < uuid2Str,
			"UUID v7 at index %d (%s) should be lexicographically smaller than UUID at index %d (%s)",
			i, uuid1Str, i+1, uuid2Str)

		assert.True(t, timestamps[i].Before(timestamps[i+1]) || timestamps[i].Equal(timestamps[i+1]),
			"Timestamp at index %d should be before or equal to timestamp at index %d", i, i+1)
	}
}

func TestCursorPaginationRealWorldScenario(t *testing.T) {
	type Item struct {
		ID        string
		Name      string
		CreatedAt time.Time
	}

	items := make([]Item, 20)
	for i := 0; i < 20; i++ {
		itemUUID, err := uuid.NewV7()
		require.NoError(t, err)
		items[i] = Item{
			ID:        itemUUID.String(),
			Name:      "Item " + itemUUID.String()[:8],
			CreatedAt: time.Now(),
		}
		time.Sleep(1 * time.Millisecond)
	}

	limit := 5

	page1Items := items[:limit]

	pagination, err := CalculateCursor(true, true, true, page1Items[0].ID, page1Items[len(page1Items)-1].ID)
	require.NoError(t, err)
	assert.NotEmpty(t, pagination.Next)
	assert.Empty(t, pagination.Prev)

	nextCursor, err := DecodeCursor(pagination.Next)
	require.NoError(t, err)
	assert.Equal(t, page1Items[len(page1Items)-1].ID, nextCursor.ID)
	assert.True(t, nextCursor.PointsNext)

	query := squirrel.Select("id", "name", "created_at").From("items")
	paginatedQuery, order := ApplyCursorPagination(query, nextCursor, "ASC", limit)

	sql, args, err := paginatedQuery.ToSql()
	require.NoError(t, err)

	expectedSQL := "SELECT id, name, created_at FROM items WHERE id > ? ORDER BY id ASC LIMIT 6"
	assert.Equal(t, expectedSQL, sql)
	assert.Equal(t, []interface{}{page1Items[len(page1Items)-1].ID}, args)
	assert.Equal(t, "ASC", order)
}

func TestLastPageScenario(t *testing.T) {
	uuids := make([]uuid.UUID, 5)
	for i := 0; i < 5; i++ {
		var err error
		uuids[i], err = uuid.NewV7()
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond)
	}

	items := make([]string, len(uuids))
	for i, u := range uuids {
		items[i] = u.String()
	}

	limit := 3
	lastPageItems := items[limit-1:]

	isFirstPage := false
	hasPagination := false
	pointsNext := true

	pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, lastPageItems[0], lastPageItems[len(lastPageItems)-1])
	require.NoError(t, err)

	assert.Empty(t, pagination.Next, "Last page should not have next_cursor")
	assert.NotEmpty(t, pagination.Prev, "Last page should have prev_cursor")

	decodedPrev, err := DecodeCursor(pagination.Prev)
	require.NoError(t, err)
	assert.Equal(t, lastPageItems[0], decodedPrev.ID)
	assert.False(t, decodedPrev.PointsNext)
}

func TestNavigationFromSecondPageBackToFirst(t *testing.T) {
	uuids := make([]uuid.UUID, 5)
	for i := 0; i < 5; i++ {
		var err error
		uuids[i], err = uuid.NewV7()
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond)
	}

	items := make([]string, len(uuids))
	for i, u := range uuids {
		items[i] = u.String()
	}

	limit := 3

	t.Run("simulate second page", func(t *testing.T) {
		secondPageItems := items[1 : limit+1]

		isFirstPage := false
		hasPagination := len(items) > limit
		pointsNext := true

		pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, secondPageItems[0], secondPageItems[len(secondPageItems)-1])
		require.NoError(t, err)

		assert.NotEmpty(t, pagination.Next, "Second page should have next_cursor")
		assert.NotEmpty(t, pagination.Prev, "Second page should have prev_cursor")
	})

	t.Run("navigate back to first page using prev_cursor", func(t *testing.T) {
		firstPageItemsFromPrev := items[:limit]

		isFirstPage := true
		hasPagination := len(items) > limit
		pointsNext := false

		pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, firstPageItemsFromPrev[0], firstPageItemsFromPrev[len(firstPageItemsFromPrev)-1])
		require.NoError(t, err)

		assert.NotEmpty(t, pagination.Next, "When returning to first page via prev, should have next_cursor")
		assert.Empty(t, pagination.Prev, "When returning to first page via prev, should NOT have prev_cursor - first page never has prev")

		decodedNext, err := DecodeCursor(pagination.Next)
		require.NoError(t, err)
		assert.Equal(t, firstPageItemsFromPrev[len(firstPageItemsFromPrev)-1], decodedNext.ID)
		assert.True(t, decodedNext.PointsNext)
	})
}

func TestCompleteNavigationFlow(t *testing.T) {
	uuids := make([]uuid.UUID, 7)
	for i := 0; i < 7; i++ {
		var err error
		uuids[i], err = uuid.NewV7()
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond)
	}

	items := make([]string, len(uuids))
	for i, u := range uuids {
		items[i] = u.String()
	}

	limit := 3

	t.Run("first page - initial load", func(t *testing.T) {
		firstPageItems := items[:limit]

		isFirstPage := true
		hasPagination := len(items) > limit
		pointsNext := true

		pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, firstPageItems[0], firstPageItems[len(firstPageItems)-1])
		require.NoError(t, err)

		assert.NotEmpty(t, pagination.Next, "First page should have next_cursor")
		assert.Empty(t, pagination.Prev, "First page should NOT have prev_cursor")
	})

	t.Run("second page - using next_cursor", func(t *testing.T) {
		secondPageItems := items[limit : limit*2]

		isFirstPage := false
		hasPagination := len(items) > limit*2
		pointsNext := true

		pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, secondPageItems[0], secondPageItems[len(secondPageItems)-1])
		require.NoError(t, err)

		assert.NotEmpty(t, pagination.Next, "Second page should have next_cursor")
		assert.NotEmpty(t, pagination.Prev, "Second page should have prev_cursor")
	})

	t.Run("last page - using next_cursor", func(t *testing.T) {
		lastPageItems := items[limit*2:]

		isFirstPage := false
		hasPagination := false
		pointsNext := true

		pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, lastPageItems[0], lastPageItems[len(lastPageItems)-1])
		require.NoError(t, err)

		assert.Empty(t, pagination.Next, "Last page should NOT have next_cursor")
		assert.NotEmpty(t, pagination.Prev, "Last page should have prev_cursor")
	})

	t.Run("back to second page - using prev_cursor", func(t *testing.T) {
		secondPageItems := items[limit : limit*2]

		isFirstPage := false
		hasPagination := len(items) > limit
		pointsNext := false

		pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, secondPageItems[0], secondPageItems[len(secondPageItems)-1])
		require.NoError(t, err)

		assert.NotEmpty(t, pagination.Next, "Second page (via prev) should have next_cursor")
		assert.NotEmpty(t, pagination.Prev, "Second page (via prev) should have prev_cursor")
	})

	t.Run("back to first page - using prev_cursor", func(t *testing.T) {
		firstPageItems := items[:limit]

		isFirstPage := true
		hasPagination := len(items) > limit
		pointsNext := false

		pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, firstPageItems[0], firstPageItems[len(firstPageItems)-1])
		require.NoError(t, err)

		assert.NotEmpty(t, pagination.Next, "First page (via prev) should have next_cursor")
		assert.Empty(t, pagination.Prev, "First page (via prev) should NOT have prev_cursor - first page never has prev")
	})
}

func TestPaginationEdgeCases(t *testing.T) {
	t.Run("single page - no pagination needed", func(t *testing.T) {
		uuid1, err := uuid.NewV7()
		require.NoError(t, err)

		items := []string{uuid1.String()}

		isFirstPage := true
		hasPagination := false
		pointsNext := true

		pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, items[0], items[0])
		require.NoError(t, err)

		assert.Empty(t, pagination.Next, "Single page should not have next_cursor")
		assert.Empty(t, pagination.Prev, "Single page should not have prev_cursor")
	})

	t.Run("exactly two pages", func(t *testing.T) {
		uuids := make([]uuid.UUID, 4)
		for i := 0; i < 4; i++ {
			var err error
			uuids[i], err = uuid.NewV7()
			require.NoError(t, err)
			time.Sleep(1 * time.Millisecond)
		}

		items := make([]string, len(uuids))
		for i, u := range uuids {
			items[i] = u.String()
		}

		limit := 2

		firstPageItems := items[:limit]
		isFirstPage := true
		hasPagination := len(items) > limit
		pointsNext := true

		pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, firstPageItems[0], firstPageItems[len(firstPageItems)-1])
		require.NoError(t, err)

		assert.NotEmpty(t, pagination.Next, "First page of two should have next_cursor")
		assert.Empty(t, pagination.Prev, "First page of two should not have prev_cursor")

		lastPageItems := items[limit:]
		isFirstPage = false
		hasPagination = false
		pointsNext = true

		pagination, err = CalculateCursor(isFirstPage, hasPagination, pointsNext, lastPageItems[0], lastPageItems[len(lastPageItems)-1])
		require.NoError(t, err)

		assert.Empty(t, pagination.Next, "Last page of two should not have next_cursor")
		assert.NotEmpty(t, pagination.Prev, "Last page of two should have prev_cursor")
	})
}

func TestBugReproduction(t *testing.T) {
	t.Run("REAL bug reproduction: repository implementation", func(t *testing.T) {
		uuids := make([]uuid.UUID, 3)
		for i := 0; i < 3; i++ {
			var err error
			uuids[i], err = uuid.NewV7()
			require.NoError(t, err)
			time.Sleep(1 * time.Millisecond)
		}

		items := make([]string, len(uuids))
		for i, u := range uuids {
			items[i] = u.String()
		}

		limit := 2

		t.Run("step 1: first page initial load (cursor empty)", func(t *testing.T) {
			cursor := ""
			allResults := append(items[:limit], "dummy_item")

			isFirstPage := cursor == ""
			hasPagination := len(allResults) > limit
			pointsNext := true
			actualResults := allResults[:limit]

			pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, actualResults[0], actualResults[len(actualResults)-1])
			require.NoError(t, err)

			t.Logf("First page initial: next=%s, prev=%s", pagination.Next, pagination.Prev)
			assert.NotEmpty(t, pagination.Next, "Initial first page should have next_cursor")
			assert.Empty(t, pagination.Prev, "Initial first page should NOT have prev_cursor")
		})

		t.Run("step 2: second page using next_cursor", func(t *testing.T) {
			firstPageCursor := CreateCursor(items[1], true)
			cursorBytes, _ := json.Marshal(firstPageCursor)
			cursor := base64.StdEncoding.EncodeToString(cursorBytes)

			decodedCursor, _ := DecodeCursor(cursor)
			allResults := items[1:]

			isFirstPage := false
			hasPagination := false
			pointsNext := decodedCursor.PointsNext
			actualResults := allResults

			if len(actualResults) > limit {
				actualResults = actualResults[:limit]
			}

			pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, actualResults[0], actualResults[len(actualResults)-1])
			require.NoError(t, err)

			t.Logf("Second page: next=%s, prev=%s", pagination.Next, pagination.Prev)
			assert.Empty(t, pagination.Next, "Second page (last) should not have next_cursor")
			assert.NotEmpty(t, pagination.Prev, "Second page should have prev_cursor")
		})

		t.Run("step 3: back to first page using prev_cursor - CORRECT", func(t *testing.T) {
			prevPageCursor := CreateCursor(items[1], false)
			cursorBytes, _ := json.Marshal(prevPageCursor)
			cursor := base64.StdEncoding.EncodeToString(cursorBytes)

			decodedCursor, _ := DecodeCursor(cursor)
			firstPageItems := items[:limit]

			isFirstPage := len(firstPageItems) < limit || firstPageItems[0] == items[0]
			hasPagination := len(items) > limit
			pointsNext := decodedCursor.PointsNext

			pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, firstPageItems[0], firstPageItems[len(firstPageItems)-1])
			require.NoError(t, err)

			t.Logf("Back to first page: isFirstPage=%v, hasPagination=%v, pointsNext=%v", isFirstPage, hasPagination, pointsNext)
			t.Logf("Back to first page result: next=%s, prev=%s", pagination.Next, pagination.Prev)
			
			assert.NotEmpty(t, pagination.Next, "First page (back from prev) should have next_cursor")
			assert.Empty(t, pagination.Prev, "First page (back from prev) should NOT have prev_cursor - CORRECT")
		})

		t.Run("step 3: back to first page - WRONG IMPLEMENTATION (YOUR BUG)", func(t *testing.T) {
			prevPageCursor := CreateCursor(items[1], false)
			cursorBytes, _ := json.Marshal(prevPageCursor)
			cursor := base64.StdEncoding.EncodeToString(cursorBytes)

			decodedCursor, _ := DecodeCursor(cursor)
			firstPageItems := items[:limit]

			isFirstPage := false
			hasPagination := len(items) > limit
			pointsNext := decodedCursor.PointsNext

			pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, firstPageItems[0], firstPageItems[len(firstPageItems)-1])
			require.NoError(t, err)

			t.Logf("WRONG: Back to first page: isFirstPage=%v, hasPagination=%v, pointsNext=%v", isFirstPage, hasPagination, pointsNext)
			t.Logf("WRONG: Back to first page result: next=%s, prev=%s", pagination.Next, pagination.Prev)
			
			assert.NotEmpty(t, pagination.Next, "First page should have next_cursor")
			assert.NotEmpty(t, pagination.Prev, "BUG: First page incorrectly has prev_cursor because isFirstPage=false")
		})
	})

	t.Run("bug: infinite loop with same cursor values", func(t *testing.T) {
		firstItemID := "0198c376-87de-7234-a8da-8e6ec327889d"
		lastItemID := "0198c376-2a4b-74e5-a25a-2777b1a87ab9"

		isFirstPage := false
		hasPagination := true
		pointsNext := false

		pagination, err := CalculateCursor(isFirstPage, hasPagination, pointsNext, firstItemID, lastItemID)
		require.NoError(t, err)

		if pagination.Next != "" && pagination.Prev != "" {
			nextCursor, err := DecodeCursor(pagination.Next)
			require.NoError(t, err)
			prevCursor, err := DecodeCursor(pagination.Prev)
			require.NoError(t, err)

			assert.NotEqual(t, nextCursor.ID, prevCursor.ID, "Next and Prev cursors should point to different IDs to avoid infinite loops")
			assert.True(t, nextCursor.PointsNext, "Next cursor should have PointsNext=true")
			assert.False(t, prevCursor.PointsNext, "Prev cursor should have PointsNext=false")
		}
	})
}
