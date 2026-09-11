// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const balanceValidationEntity = "validateBalance"

type engineTechnicalError interface {
	error
	EngineFailureCode() string
	OutcomeIndeterminate() bool
}

// MapEngineError translates deterministic engine failures to the
// legacy public contract. Technical and malformed engine errors retain their
// cause for classification at the caller's error boundary.
func MapEngineError(request accounting.Execution, err error) error {
	if err == nil {
		return nil
	}

	var technicalErr engineTechnicalError
	if errors.As(err, &technicalErr) {
		if technicalErr.EngineFailureCode() == "execution_guard_conflict" && !technicalErr.OutcomeIndeterminate() {
			return pkg.ValidateBusinessError(constant.ErrPendingTransactionLocked, balanceValidationEntity)
		}

		return fmt.Errorf("engine technical failure: %w", err)
	}

	var failure *accounting.Failure
	if !errors.As(err, &failure) {
		return fmt.Errorf("engine failure: %w", err)
	}

	if failure.PostingIndex == -1 {
		requirement, balance, ok := engineFailureRequirement(request, failure)
		if !ok {
			return fmt.Errorf("malformed engine requirement failure: %w", err)
		}

		switch failure.Code {
		case accounting.FailureAssetMismatch:
			entity := "validateFromAccounts"
			if requirement.Permission == accounting.BalancePermissionReceive {
				entity = "validateToAccounts"
			}

			return pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, entity)
		case accounting.FailureSendingNotAllowed:
			if requirement.Permission != accounting.BalancePermissionSend {
				return fmt.Errorf("malformed engine sending requirement failure: %w", err)
			}

			return pkg.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateFromAccounts")
		case accounting.FailureReceivingNotAllowed:
			if requirement.Permission != accounting.BalancePermissionReceive {
				return fmt.Errorf("malformed engine receiving requirement failure: %w", err)
			}

			return pkg.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateToAccounts")
		case accounting.FailureExternalHoldNotAllowed:
			if !requirement.ForbidExternal {
				return fmt.Errorf("malformed engine external hold requirement failure: %w", err)
			}

			return pkg.ValidateBusinessError(constant.ErrOnHoldExternalAccount, balanceValidationEntity, balance.Alias)
		case accounting.FailureBalanceDeleted:
			return pkg.ValidateBusinessError(constant.ErrAccountIneligibility, balanceValidationEntity)
		default:
			return fmt.Errorf("unexpected engine requirement failure: %w", err)
		}
	}

	posting, ok := engineFailurePosting(request, failure)
	if !ok {
		return fmt.Errorf("malformed engine failure: %w", err)
	}

	switch failure.Code {
	case "insufficient_funds":
		return pkg.ValidateBusinessError(constant.ErrInsufficientFunds, balanceValidationEntity)
	case "overdraft_limit_exceeded":
		return pkg.ValidateBusinessError(constant.ErrOverdraftLimitExceeded, balanceValidationEntity)
	case "overdraft_not_eligible":
		switch posting.DrawPolicy {
		case accounting.DrawRouteDenied:
			return pkg.ValidateBusinessError(constant.ErrOverdraftRouteNotConfigured, balanceValidationEntity)
		case accounting.DrawForbidden:
			return pkg.ValidateBusinessError(constant.ErrInsufficientFunds, balanceValidationEntity)
		default:
			return fmt.Errorf("unexpected draw policy for engine failure: %w", err)
		}
	case "overdraft_companion_missing":
		return fmt.Errorf("overdraft companion missing: %w", err)
	case "balance_deleted":
		return pkg.ValidateBusinessError(constant.ErrAccountIneligibility, balanceValidationEntity)
	case "balance_missing":
		return pkg.ValidateBusinessError(constant.ErrTransactionBackupCacheRetrievalFailed, balanceValidationEntity)
	case "onhold_underflow":
		return fmt.Errorf("on-hold balance underflow: %w", err)
	default:
		return fmt.Errorf("unknown engine failure: %w", err)
	}
}

func engineFailureRequirement(request accounting.Execution, failure *accounting.Failure) (accounting.BalanceRequirement, accounting.BalanceSnapshot, bool) {
	if failure == nil || failure.TransactionIndex < 0 || failure.TransactionIndex >= len(request.Transactions) || failure.BalanceRef == "" {
		return accounting.BalanceRequirement{}, accounting.BalanceSnapshot{}, false
	}

	var requirement *accounting.BalanceRequirement
	for i := range request.Transactions[failure.TransactionIndex].BalanceRequirements {
		candidate := &request.Transactions[failure.TransactionIndex].BalanceRequirements[i]
		if candidate.BalanceRef == failure.BalanceRef {
			requirement = candidate
			break
		}
	}

	if requirement == nil {
		return accounting.BalanceRequirement{}, accounting.BalanceSnapshot{}, false
	}

	for _, balance := range request.Balances {
		if balance.BalanceRef == failure.BalanceRef {
			return *requirement, balance, true
		}
	}

	return accounting.BalanceRequirement{}, accounting.BalanceSnapshot{}, false
}

func engineFailurePosting(request accounting.Execution, failure *accounting.Failure) (accounting.Posting, bool) {
	if failure == nil || failure.TransactionIndex < 0 || failure.TransactionIndex >= len(request.Transactions) {
		return accounting.Posting{}, false
	}

	transaction := request.Transactions[failure.TransactionIndex]
	if failure.PostingIndex < 0 || failure.PostingIndex >= len(transaction.Postings) {
		return accounting.Posting{}, false
	}

	posting := transaction.Postings[failure.PostingIndex]
	if failure.BalanceRef == "" {
		return accounting.Posting{}, false
	}

	if failure.BalanceRef != posting.BalanceRef && !isOverdraftCompanion(request, posting.BalanceRef, failure.BalanceRef) {
		return accounting.Posting{}, false
	}

	return posting, true
}

func isOverdraftCompanion(request accounting.Execution, postingRef, failureRef string) bool {
	var origin *accounting.BalanceSnapshot

	for i := range request.Balances {
		if request.Balances[i].BalanceRef == postingRef {
			origin = &request.Balances[i]
			break
		}
	}

	if origin == nil || origin.AccountID == uuid.Nil {
		return false
	}

	for i := range request.Balances {
		balance := request.Balances[i]
		if balance.BalanceRef == failureRef && balance.AccountID == origin.AccountID && balance.Key == "overdraft" {
			return true
		}
	}

	return false
}
