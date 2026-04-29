// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/pagination"
	"github.com/Masterminds/squirrel"
)

func applyCursorPagination(findAll squirrel.SelectBuilder, decodedCursor libHTTP.Cursor, orderDirection string, limit int) (squirrel.SelectBuilder, error) {
	return pagination.ApplyCursorPagination(findAll, decodedCursor, orderDirection, limit)
}
