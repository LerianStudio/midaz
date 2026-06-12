// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/google/uuid"
)

// LedgerAccountReader is the narrow port CreateInstrument uses to verify the
// body-supplied ledger and account references exist within the request
// organization before persisting the instrument.
//
// It is defined here, in the CRM services package, on purpose: CRM must not
// import the ledger query package (dependency-inward). The implementation (an
// adapter over the ledger query use case, wired at bootstrap) translates the
// underlying repository's not-found into a sentinel-free signal; this contract
// reports existence as a boolean so the use case maps absence to the 422
// referential sentinels rather than surfacing the query layer's 404-typed
// errors.
type LedgerAccountReader interface {
	// LedgerExists reports whether a ledger with ledgerID exists within the
	// organization. A not-found is reported as (false, nil); every other error
	// propagates so transient/infrastructure failures do not masquerade as
	// absence.
	LedgerExists(ctx context.Context, organizationID, ledgerID uuid.UUID) (bool, error)
	// AccountExists reports whether an account with accountID exists within the
	// ledger. Absence is (false, nil); other errors propagate.
	AccountExists(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (bool, error)
}
