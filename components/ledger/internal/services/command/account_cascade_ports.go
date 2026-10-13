// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// InstrumentCascader is the narrow port the account delete uses to soft-delete the
// CRM instruments linked to the account before the account row goes away.
//
// It is defined here, in the command package, on purpose: command must not import
// the CRM package (dependency-inward). The implementation is an adapter over the
// CRM instrument service, wired at bootstrap, that resolves the tenant's CRM store.
// Errors it returns are technical: a business outcome such as "no live instrument"
// is a zero count, never an error.
type InstrumentCascader interface {
	// SoftDeleteInstrumentsByAccount soft-deletes every live instrument linked to the
	// account within the organization and ledger and reports how many it deleted.
	SoftDeleteInstrumentsByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (int, error)
}

// FeeAliasDetacher is the narrow port the account delete uses to remove the
// account's alias from the ledger's fee and billing packages before the account
// row goes away.
//
// Like InstrumentCascader it keeps command free of the fees service package; the
// implementation is an adapter over the fees use case, wired at bootstrap, that
// resolves the tenant's fees store. Errors it returns are technical: a ledger with
// no referencing package is a zero result, never an error.
type FeeAliasDetacher interface {
	// DetachAccountAlias removes every reference to alias from the ledger's fee and
	// billing packages and reports how many packages it updated and disabled.
	DetachAccountAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (model.FeeAliasDetachResult, error)
}
