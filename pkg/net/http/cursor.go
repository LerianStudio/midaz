package http

import (
	"encoding/base64"
	"encoding/json"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/Masterminds/squirrel"
	"strings"
)

type Cursor struct {
	ID         string `json:"id"`
	PointsNext bool   `json:"points_next"`
}

// CursorPagination entity to store cursor pagination to return to client
type CursorPagination struct {
	Next string `json:"next"`
	Prev string `json:"prev"`
}

// CreateCursor creates a cursor encode struct.
func CreateCursor(id string, pointsNext bool) Cursor {
	cursor := Cursor{
		ID:         id,
		PointsNext: pointsNext,
	}

	return cursor
}

// DecodeCursor decodes a cursor string.
func DecodeCursor(cursor string) (Cursor, error) {
	decodedCursor, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return Cursor{}, err
	}

	var cur Cursor

	if err := json.Unmarshal(decodedCursor, &cur); err != nil {
		return Cursor{}, err
	}

	return cur, nil
}

// ApplyCursorPagination applies cursor-based pagination to a query.
func ApplyCursorPagination(findAll squirrel.SelectBuilder, decodedCursor Cursor, orderDirection string, limit int) (squirrel.SelectBuilder, string) {
	var operator string

	var sortOrder string

	ascOrder := strings.ToUpper(string(constant.Asc))
	descOrder := strings.ToUpper(string(constant.Desc))

	if decodedCursor.ID != "" {
		pointsNext := decodedCursor.PointsNext

		if pointsNext && orderDirection == ascOrder {
			operator = ">"
			sortOrder = ascOrder
		}

		if pointsNext && orderDirection == descOrder {
			operator = "<"
			sortOrder = descOrder
		}

		if !pointsNext && orderDirection == ascOrder {
			operator = "<"
			sortOrder = descOrder
		}

		if !pointsNext && orderDirection == descOrder {
			operator = ">"
			sortOrder = ascOrder
		}

		whereClause := squirrel.Expr("id "+operator+" ?", decodedCursor.ID)

		// Forward pagination with DESC order
		findAll = findAll.Where(whereClause).
			OrderBy("id " + sortOrder)

		return findAll.Limit(pkg.SafeIntToUint64(limit + 1)), orderDirection
	}

	// No cursor means this is the first page; use the order as normal
	findAll = findAll.OrderBy("id " + orderDirection)

	return findAll.Limit(pkg.SafeIntToUint64(limit + 1)), orderDirection
}

// PaginateRecords paginates records based on the cursor.
func PaginateRecords[T any](isFirstPage bool, hasPagination bool, pointsNext bool, items []T, limit int, orderDirection string) []T {
	paginatedItems := items

	if isFirstPage {
		if hasPagination {
			return paginatedItems[:limit]
		}

		return paginatedItems
	}

	if pointsNext {
		if hasPagination {
			return paginatedItems[:limit]
		}

		return paginatedItems
	}

	if hasPagination {
		paginatedItems = paginatedItems[:limit]
	}

	return pkg.Reverse(paginatedItems)
}

// CalculateCursor calculates the cursor pagination.
func CalculateCursor(isFirstPage, hasPagination, pointsNext bool, firstItemID, lastItemID string) (CursorPagination, error) {
	prevCur := Cursor{}
	nextCur := Cursor{}
	pagination := CursorPagination{}

	if isFirstPage {
		if hasPagination {
			nextCur = CreateCursor(lastItemID, true)
		}
	} else {
		if pointsNext {
			if hasPagination {
				nextCur = CreateCursor(lastItemID, true)
			}

			prevCur = CreateCursor(firstItemID, false)
		} else {
			nextCur = CreateCursor(lastItemID, true)

			if hasPagination {
				prevCur = CreateCursor(firstItemID, false)
			}
		}
	}

	if !pkg.IsNilOrEmpty(&prevCur.ID) {
		serializedPrevCursor, err := json.Marshal(prevCur)
		if err != nil {
			return CursorPagination{}, err
		}

		pagination.Prev = base64.StdEncoding.EncodeToString(serializedPrevCursor)
	}

	if !pkg.IsNilOrEmpty(&nextCur.ID) {
		serializedNextCursor, err := json.Marshal(nextCur)
		if err != nil {
			return CursorPagination{}, err
		}

		pagination.Next = base64.StdEncoding.EncodeToString(serializedNextCursor)
	}

	return pagination, nil
}
