package assetrate

import (
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/pagination"
	"github.com/Masterminds/squirrel"
)

func applyCursorPagination(findAll squirrel.SelectBuilder, decodedCursor libHTTP.Cursor, orderDirection string, limit int) (squirrel.SelectBuilder, error) {
	return pagination.ApplyCursorPagination(findAll, decodedCursor, orderDirection, limit)
}
