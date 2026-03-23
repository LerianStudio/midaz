// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	ledgerServices "github.com/LerianStudio/midaz/v3/components/ledger/services"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDatabaseItemNotFound is thrown when an item informed was not found.
// Re-exported from ledger/services to ensure sentinel identity across components.
var ErrDatabaseItemNotFound = ledgerServices.ErrDatabaseItemNotFound

// ValidatePGError validates pgError and returns the appropriate business error.
// Delegates to the unified implementation in ledger/services.
func ValidatePGError(pgErr *pgconn.PgError, entityType string, args ...any) error {
	return ledgerServices.ValidatePGError(pgErr, entityType, args...)
}
