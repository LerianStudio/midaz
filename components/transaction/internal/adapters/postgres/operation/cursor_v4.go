package operation

import (
	"fmt"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	cn "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	"github.com/Masterminds/squirrel"
)

func applyCursorPagination(findAll squirrel.SelectBuilder, decodedCursor libHTTP.Cursor, orderDirection string, limit int) (squirrel.SelectBuilder, error) {
	normalizedOrder := strings.ToUpper(strings.TrimSpace(orderDirection))
	if normalizedOrder != cn.SortDirASC && normalizedOrder != cn.SortDirDESC {
		return findAll, fmt.Errorf("invalid sort order: %s", orderDirection)
	}

	if decodedCursor.ID != "" {
		operator, effectiveOrder, err := libHTTP.CursorDirectionRules(normalizedOrder, decodedCursor.Direction)
		if err != nil {
			return findAll, err
		}

		findAll = findAll.Where(squirrel.Expr("id "+operator+" ?", decodedCursor.ID)).OrderBy("id " + effectiveOrder)

		return findAll.Limit(libCommons.SafeIntToUint64(limit + 1)), nil
	}

	findAll = findAll.OrderBy("id " + normalizedOrder)

	return findAll.Limit(libCommons.SafeIntToUint64(limit + 1)), nil
}
