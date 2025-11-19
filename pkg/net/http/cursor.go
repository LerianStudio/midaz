package http

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/Masterminds/squirrel"
)

type Cursor struct {
	ID         string `json:"id"`
	PointsNext bool   `json:"points_next"`
}

type CursorPagination struct {
	Next string `json:"next"`
	Prev string `json:"prev"`
}

func CreateCursor(id string, pointsNext bool) Cursor {
	return Cursor{
		ID:         id,
		PointsNext: pointsNext,
	}
}

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

func ApplyCursorPagination(
	findAll squirrel.SelectBuilder,
	decodedCursor Cursor,
	orderDirection string,
	limit int,
	tableAlias ...string,
) (squirrel.SelectBuilder, string) {
	var operator string

	var actualOrder string

	ascOrder := strings.ToUpper(string(constant.Asc))
	descOrder := strings.ToUpper(string(constant.Desc))

	ID := "id"
	if len(tableAlias) > 0 {
		ID = tableAlias[0] + "." + ID
	}

	if decodedCursor.ID != "" {
		if decodedCursor.PointsNext {
			if orderDirection == ascOrder {
				operator = ">"
				actualOrder = ascOrder
			} else {
				operator = "<"
				actualOrder = descOrder
			}
		} else {
			if orderDirection == ascOrder {
				operator = "<"
				actualOrder = descOrder
			} else {
				operator = ">"
				actualOrder = ascOrder
			}
		}

		whereClause := squirrel.Expr(ID+" "+operator+" ?", decodedCursor.ID)
		findAll = findAll.Where(whereClause).OrderBy(ID + " " + actualOrder)

		return findAll.Limit(commons.SafeIntToUint64(limit + 1)), actualOrder
	}

	findAll = findAll.OrderBy(ID + " " + orderDirection)

	return findAll.Limit(commons.SafeIntToUint64(limit + 1)), orderDirection
}

func PaginateRecords[T any](
	isFirstPage bool,
	hasPagination bool,
	pointsNext bool,
	items []T,
	limit int,
	orderUsed string,
) []T {
	if !hasPagination {
		return items
	}

	paginated := items[:limit]

	if !pointsNext {
		return commons.Reverse(paginated)
	}

	return paginated
}

func CalculateCursor(
	isFirstPage, hasPagination, pointsNext bool,
	firstItemID, lastItemID string,
) (CursorPagination, error) {
	var pagination CursorPagination

	if pointsNext {
		if hasPagination {
			next := CreateCursor(lastItemID, true)

			cursorBytes, err := json.Marshal(next)
			if err != nil {
				return CursorPagination{}, err
			}

			pagination.Next = base64.StdEncoding.EncodeToString(cursorBytes)
		}

		if !isFirstPage {
			prev := CreateCursor(firstItemID, false)

			cursorBytes, err := json.Marshal(prev)
			if err != nil {
				return CursorPagination{}, err
			}

			pagination.Prev = base64.StdEncoding.EncodeToString(cursorBytes)
		}
	} else {
		if hasPagination || isFirstPage {
			next := CreateCursor(lastItemID, true)

			cursorBytesNext, err := json.Marshal(next)
			if err != nil {
				return CursorPagination{}, err
			}

			pagination.Next = base64.StdEncoding.EncodeToString(cursorBytesNext)
		}

		if !isFirstPage {
			prev := CreateCursor(firstItemID, false)

			cursorBytesPrev, err := json.Marshal(prev)
			if err != nil {
				return CursorPagination{}, err
			}

			pagination.Prev = base64.StdEncoding.EncodeToString(cursorBytesPrev)
		}
	}

	return pagination, nil
}
