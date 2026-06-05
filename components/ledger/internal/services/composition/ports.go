// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package composition

import (
	crmservices "github.com/LerianStudio/midaz/v3/components/crm/services"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
)

// Compile-time assertions that the concrete use cases satisfy the narrow
// composition ports. They break the build if F1/F2 ever change the composed
// signatures out from under the orchestrator.
var (
	_ AccountCreator    = (*command.UseCase)(nil)
	_ InstrumentCreator = (*crmservices.UseCase)(nil)
)
