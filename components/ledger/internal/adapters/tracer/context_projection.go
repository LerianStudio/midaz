// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// PreparedEntry is a fee-inclusive posting prepared before accounting executes.
// Auxiliary overdraft balance movements are not additional prepared entries.
type PreparedEntry struct {
	AccountID uuid.UUID
	External  bool
	Direction tracercontract.Direction
	Amount    decimal.Decimal
	AssetCode string
}

// ContextInput holds official records already resolved in the authenticated
// tenant. The use case fetches them in batches after the off/skip gates; this
// adapter only maps facts and checks their organization/ledger ownership.
// Accounts and entries carry their asset codes verbatim.
type ContextInput struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	Accounts       []*mmodel.Account
	Entries        []PreparedEntry
}

// BuildEvaluationContext projects Ledger facts without importing its models into
// the shared contract. It never computes limit consumption, maps account types
// into a fixed taxonomy, fetches data, or changes prepared postings.
func BuildEvaluationContext(ctx context.Context, input ContextInput, limits tracercontract.Limits) (tracercontract.Context, error) {
	if err := ctx.Err(); err != nil {
		return tracercontract.Context{}, err
	}

	if err := limits.Validate(); err != nil {
		return tracercontract.Context{}, err
	}

	if input.OrganizationID == uuid.Nil || input.LedgerID == uuid.Nil ||
		len(input.Accounts) > limits.MaxAccounts || len(input.Entries) > limits.MaxEntries {
		return tracercontract.Context{}, projectionError("scope or context size")
	}

	result := tracercontract.Context{
		Accounts: make([]tracercontract.Account, 0, len(input.Accounts)),
		Entries:  make([]tracercontract.Entry, 0, len(input.Entries)),
	}

	for _, account := range input.Accounts {
		if err := ctx.Err(); err != nil {
			return tracercontract.Context{}, err
		}

		projected, err := projectAccount(account, input)
		if err != nil {
			return tracercontract.Context{}, err
		}

		result.Accounts = append(result.Accounts, projected)
	}

	for _, entry := range input.Entries {
		amount, err := tracercontract.AmountFromDecimal(ctx, entry.Amount, limits)
		if err != nil {
			return tracercontract.Context{}, err
		}

		result.Entries = append(result.Entries, tracercontract.Entry{
			AccountID: entry.AccountID, External: entry.External, Direction: entry.Direction,
			Amount: amount, Asset: entry.AssetCode,
		})
	}

	if err := result.Validate(ctx, limits); err != nil {
		return tracercontract.Context{}, err
	}

	return result, nil
}

func projectAccount(account *mmodel.Account, input ContextInput) (tracercontract.Account, error) {
	if account == nil || account.DeletedAt != nil || account.Blocked == nil || !sameScope(account.OrganizationID, account.LedgerID, input) {
		return tracercontract.Account{}, projectionError("incomplete account or account outside official scope")
	}

	id, err := uuid.Parse(account.ID)
	if err != nil || id == uuid.Nil {
		return tracercontract.Account{}, projectionError("invalid account UUID")
	}

	blocked := *account.Blocked

	return tracercontract.Account{
		ID: id, Type: account.Type, Status: account.Status.Code, Blocked: &blocked, Asset: account.AssetCode,
	}, nil
}

func sameScope(organizationID, ledgerID string, input ContextInput) bool {
	org, orgErr := uuid.Parse(organizationID)
	ledger, ledgerErr := uuid.Parse(ledgerID)

	return orgErr == nil && ledgerErr == nil && org == input.OrganizationID && ledger == input.LedgerID
}

func projectionError(field string) error {
	return fmt.Errorf("tracer context %s: %w", field, constant.ErrInvalidRequestBody)
}
