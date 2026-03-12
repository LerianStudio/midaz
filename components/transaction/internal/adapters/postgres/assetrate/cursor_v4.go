package assetrate

import (
	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	"github.com/Masterminds/squirrel"
)

func applyCursorPagination(findAll squirrel.SelectBuilder, decodedCursor libHTTP.Cursor, orderDirection string, limit int) (squirrel.SelectBuilder, error) {
	if decodedCursor.ID != "" {
		operator, effectiveOrder, err := libHTTP.CursorDirectionRules(orderDirection, decodedCursor.Direction)
		if err != nil {
			return findAll, err
		}

		findAll = findAll.Where(squirrel.Expr("id "+operator+" ?", decodedCursor.ID)).OrderBy("id " + effectiveOrder)

		return findAll.Limit(libCommons.SafeIntToUint64(limit + 1)), nil
	}

	findAll = findAll.OrderBy("id " + orderDirection)

	return findAll.Limit(libCommons.SafeIntToUint64(limit + 1)), nil
}
