// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mbootstrap

import (
	"context"

	"github.com/google/uuid"
)

//go:generate mockgen --destination=settings_mock.go --package=mbootstrap . SettingsPort

// SettingsPort defines the interface for ledger settings operations.
// This is a transport-agnostic "port" that abstracts
// how the transaction module communicates with the onboarding module for settings.
//
// This interface is implemented by:
//   - onboarding query.UseCase: Direct implementation (unified ledger mode)
//   - GRPCSettingsAdapter: Network calls via gRPC (separate services mode, future)
//
// The transaction module's UseCase can optionally depend on this port to query
// ledger settings during transaction validation.
//
// In unified ledger mode, the onboarding query.UseCase is passed directly to transaction,
// eliminating the need for intermediate adapters and enabling zero-overhead in-process calls.
type SettingsPort interface {
	// GetLedgerSettings retrieves the settings for a specific ledger.
	// Returns the settings map if the ledger exists.
	// Returns an empty map {} if the ledger exists but has no settings defined.
	// Returns an error if the ledger does not exist.
	GetLedgerSettings(ctx context.Context, organizationID, ledgerID uuid.UUID) (map[string]any, error)
}

