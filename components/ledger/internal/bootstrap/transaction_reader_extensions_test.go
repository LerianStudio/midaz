// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
)

// The command use case discovers these optional TransactionReader extensions by
// type assertion on the query use case wired as its TransactionReader, so a
// signature drift would only surface as a runtime "not configured" error.
var (
	_ command.TransactionRouteCacheReader    = (*query.UseCase)(nil)
	_ command.TransactionGroupMemberResolver = (*query.UseCase)(nil)
	_ command.GroupAccountingRouteValidator  = (*query.UseCase)(nil)
)
