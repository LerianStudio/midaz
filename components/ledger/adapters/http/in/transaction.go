// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v3/components/ledger/services/command"
	"github.com/LerianStudio/midaz/v3/components/ledger/services/query"
)

type TransactionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}
