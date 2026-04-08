// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// transactionPathParams holds the IDs extracted from URL path parameters.
// TransactionID is uuid.Nil when the route has no :transaction_id segment.
type transactionPathParams struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	TransactionID  uuid.UUID
}

// readPathParams extracts organization, ledger, and (optional) transaction
// IDs from Fiber locals populated by the UUID-parsing middleware.
func readPathParams(c *fiber.Ctx) (*transactionPathParams, error) {
	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return nil, err
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return nil, err
	}

	transactionID := uuid.Nil
	if c.Locals("transaction_id") != nil {
		transactionID, err = http.GetUUIDFromLocals(c, "transaction_id")
		if err != nil {
			return nil, err
		}
	}

	return &transactionPathParams{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		TransactionID:  transactionID,
	}, nil
}

// buildParentTransactionID converts a parent UUID to a string pointer,
// returning nil when the parent is uuid.Nil (no parent).
func buildParentTransactionID(parentID uuid.UUID) *string {
	if parentID == uuid.Nil {
		return nil
	}

	s := parentID.String()

	return &s
}

// getAliasWithoutKey strips the "#key" suffix from alias strings,
// returning only the alias portion before the first "#".
func getAliasWithoutKey(array []string) []string {
	result := make([]string, len(array))

	for i, str := range array {
		parts := strings.Split(str, "#")
		result[i] = parts[0]
	}

	return result
}

// handleAccountFields transforms account aliases in FromTo entries.
// When isConcat is true, aliases are prefixed with index and balance key for
// unique lookup (e.g. "0#@person1#default"). When false, the prefixed alias
// is split back to the original form.
func handleAccountFields(entries []pkgTransaction.FromTo, isConcat bool) []pkgTransaction.FromTo {
	result := make([]pkgTransaction.FromTo, 0, len(entries))

	for i := range entries {
		var newAlias string
		if isConcat {
			newAlias = entries[i].ConcatAlias(i)
		} else {
			newAlias = entries[i].SplitAlias()
		}

		entries[i].AccountAlias = newAlias

		result = append(result, entries[i])
	}

	return result
}

// applyDefaultBalanceKeys sets the balance key to "default" for any entry
// where the caller did not specify one. Midaz supports multiple balances per
// account (e.g. "default", "asset-freeze", "rewards"), each tracked independently.
// When the client omits the key, operations target the primary "default" balance.
func applyDefaultBalanceKeys(entries []pkgTransaction.FromTo) {
	for i := range entries {
		if entries[i].BalanceKey == "" {
			entries[i].BalanceKey = constant.DefaultBalanceKey
		}
	}
}

// checkTransactionDate validates and resolves the transaction date.
// Returns time.Now() when no date is provided. Rejects future dates
// and dates combined with pending status.
func checkTransactionDate(ctx context.Context, transactionInput pkgTransaction.Transaction, transactionStatus string) (time.Time, error) {
	now := time.Now()

	if transactionInput.TransactionDate == nil || transactionInput.TransactionDate.IsZero() {
		return now, nil
	}

	logger, _, _, _ := libCommons.NewTrackingFromContext(ctx)

	if transactionInput.TransactionDate.After(now) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidFutureTransactionDate, constant.EntityTransaction)
		logger.Log(ctx, libLog.LevelWarn, "Transaction date cannot be a future date", libLog.Err(err))

		return time.Time{}, err
	}

	if transactionStatus == constant.PENDING {
		err := pkg.ValidateBusinessError(constant.ErrInvalidPendingFutureTransactionDate, constant.EntityTransaction)
		logger.Log(ctx, libLog.LevelWarn, "Pending transaction cannot have a custom transaction date", libLog.Err(err))

		return time.Time{}, err
	}

	return transactionInput.TransactionDate.Time(), nil
}
