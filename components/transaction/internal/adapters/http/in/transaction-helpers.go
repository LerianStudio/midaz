// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// deriveTransactionAction returns the action constant based on whether
// the transaction is pending (hold) or immediate (direct).
func deriveTransactionAction(pending bool) string {
	if pending {
		return constant.ActionHold
	}

	return constant.ActionDirect
}

// deriveCommitCancelAction returns the action constant based on the
// transaction status for commit/cancel operations.
func deriveCommitCancelAction(status string) string {
	if status == constant.APPROVED {
		return constant.ActionCommit
	}

	return constant.ActionCancel
}

// deriveRevertAction returns the action constant for revert operations.
func deriveRevertAction() string {
	return constant.ActionRevert
}

// resolveAccountingEntry returns the AccountingEntry matching the given action
// from the operation route's AccountingEntries. Returns nil if entries is nil
// or the action has no configured entry.
func resolveAccountingEntry(action string, accountingEntries *mmodel.AccountingEntries) *mmodel.AccountingEntry {
	if accountingEntries == nil {
		return nil
	}

	switch action {
	case constant.ActionDirect:
		return accountingEntries.Direct
	case constant.ActionHold:
		return accountingEntries.Hold
	case constant.ActionCommit:
		return accountingEntries.Commit
	case constant.ActionCancel:
		return accountingEntries.Cancel
	case constant.ActionRevert:
		return accountingEntries.Revert
	default:
		return nil
	}
}

// resolveOperationRouteCode determines the route ID, debit rubric code, and
// credit rubric code for a given operation by:
//  1. Looking up the alias in validate.OperationRoutesFrom/To to find the routeID
//  2. Finding the route in the flat cache
//  3. Resolving the AccountingEntry for the current action (direct/hold/commit/cancel/revert)
//  4. Extracting the debit and credit rubric codes from that entry
//
// Returns (nil, nil, nil) if no route mapping exists.
func resolveOperationRouteCode(ft pkgTransaction.FromTo, validate *pkgTransaction.Responses, flatRouteCache map[string]mmodel.OperationRouteCache, action string) (*string, *string, *string) {
	if validate == nil {
		return nil, nil, nil
	}

	var routeID string

	// Check if this operation has a route mapping via the alias in From or To maps.
	// The validate.OperationRoutesFrom/To keys match the full alias used in validate.From/To.
	if ft.IsFrom {
		for alias, id := range validate.OperationRoutesFrom {
			if pkgTransaction.SplitAlias(alias) == pkgTransaction.SplitAlias(ft.AccountAlias) {
				routeID = id

				break
			}
		}
	} else {
		for alias, id := range validate.OperationRoutesTo {
			if pkgTransaction.SplitAlias(alias) == pkgTransaction.SplitAlias(ft.AccountAlias) {
				routeID = id

				break
			}
		}
	}

	if routeID == "" {
		return nil, nil, nil
	}

	route, found := flatRouteCache[routeID]
	if !found {
		return &routeID, nil, nil
	}

	entry := resolveAccountingEntry(action, route.AccountingEntries)
	if entry == nil {
		return &routeID, nil, nil
	}

	var debitCode, creditCode *string

	if entry.Debit != nil && entry.Debit.Code != "" {
		debitCode = &entry.Debit.Code
	}

	if entry.Credit != nil && entry.Credit.Code != "" {
		creditCode = &entry.Credit.Code
	}

	return &routeID, debitCode, creditCode
}
