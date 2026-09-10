// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

const balanceValidationEntity = "validateBalance"

type balanceEngineTechnicalError interface {
	error
	EngineFailureCode() string
	OutcomeIndeterminate() bool
}

// MapBalanceEngineError translates deterministic balance-engine failures to the
// legacy public contract. Technical and malformed engine errors retain their
// cause for classification at the caller's error boundary.
func MapBalanceEngineError(request engine.Request, err error) error {
	if err == nil {
		return nil
	}

	var technicalErr balanceEngineTechnicalError
	if errors.As(err, &technicalErr) {
		if technicalErr.EngineFailureCode() == "execution_guard_conflict" && !technicalErr.OutcomeIndeterminate() {
			return pkg.ValidateBusinessError(constant.ErrPendingTransactionLocked, balanceValidationEntity)
		}

		return fmt.Errorf("balance engine technical failure: %w", err)
	}

	var failure *engine.Failure
	if !errors.As(err, &failure) {
		return fmt.Errorf("balance engine failure: %w", err)
	}

	if failure.PostingIndex == -1 {
		requirement, balance, ok := engineFailureRequirement(request, failure)
		if !ok {
			return fmt.Errorf("malformed balance engine requirement failure: %w", err)
		}

		switch failure.Code {
		case engine.FailureAssetMismatch:
			entity := "validateFromAccounts"
			if requirement.Permission == engine.BalancePermissionReceive {
				entity = "validateToAccounts"
			}

			return pkg.ValidateBusinessError(constant.ErrAssetCodeNotFound, entity)
		case engine.FailureSendingNotAllowed:
			if requirement.Permission != engine.BalancePermissionSend {
				return fmt.Errorf("malformed balance engine sending requirement failure: %w", err)
			}

			return pkg.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateFromAccounts")
		case engine.FailureReceivingNotAllowed:
			if requirement.Permission != engine.BalancePermissionReceive {
				return fmt.Errorf("malformed balance engine receiving requirement failure: %w", err)
			}

			return pkg.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateToAccounts")
		case engine.FailureExternalHoldNotAllowed:
			if !requirement.ForbidExternal {
				return fmt.Errorf("malformed balance engine external hold requirement failure: %w", err)
			}

			return pkg.ValidateBusinessError(constant.ErrOnHoldExternalAccount, balanceValidationEntity, balance.Alias)
		case engine.FailureBalanceDeleted:
			return pkg.ValidateBusinessError(constant.ErrAccountIneligibility, balanceValidationEntity)
		default:
			return fmt.Errorf("unexpected balance engine requirement failure: %w", err)
		}
	}

	posting, ok := engineFailurePosting(request, failure)
	if !ok {
		return fmt.Errorf("malformed balance engine failure: %w", err)
	}

	switch failure.Code {
	case "insufficient_funds":
		return pkg.ValidateBusinessError(constant.ErrInsufficientFunds, balanceValidationEntity)
	case "overdraft_limit_exceeded":
		return pkg.ValidateBusinessError(constant.ErrOverdraftLimitExceeded, balanceValidationEntity)
	case "overdraft_not_eligible":
		switch posting.DrawPolicy {
		case engine.DrawRouteDenied:
			return pkg.ValidateBusinessError(constant.ErrOverdraftRouteNotConfigured, balanceValidationEntity)
		case engine.DrawForbidden:
			return pkg.ValidateBusinessError(constant.ErrInsufficientFunds, balanceValidationEntity)
		default:
			return fmt.Errorf("unexpected draw policy for balance engine failure: %w", err)
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
		return fmt.Errorf("unknown balance engine failure: %w", err)
	}
}

func engineFailureRequirement(request engine.Request, failure *engine.Failure) (engine.BalanceRequirement, engine.BalanceSnapshot, bool) {
	if failure == nil || failure.TransactionIndex < 0 || failure.TransactionIndex >= len(request.Transactions) || failure.BalanceRef == "" {
		return engine.BalanceRequirement{}, engine.BalanceSnapshot{}, false
	}

	var requirement *engine.BalanceRequirement
	for i := range request.Transactions[failure.TransactionIndex].BalanceRequirements {
		candidate := &request.Transactions[failure.TransactionIndex].BalanceRequirements[i]
		if candidate.BalanceRef == failure.BalanceRef {
			requirement = candidate
			break
		}
	}

	if requirement == nil {
		return engine.BalanceRequirement{}, engine.BalanceSnapshot{}, false
	}

	for _, balance := range request.Balances {
		if balance.BalanceRef == failure.BalanceRef {
			return *requirement, balance, true
		}
	}

	return engine.BalanceRequirement{}, engine.BalanceSnapshot{}, false
}

func engineFailurePosting(request engine.Request, failure *engine.Failure) (engine.Posting, bool) {
	if failure == nil || failure.TransactionIndex < 0 || failure.TransactionIndex >= len(request.Transactions) {
		return engine.Posting{}, false
	}

	transaction := request.Transactions[failure.TransactionIndex]
	if failure.PostingIndex < 0 || failure.PostingIndex >= len(transaction.Postings) {
		return engine.Posting{}, false
	}

	posting := transaction.Postings[failure.PostingIndex]
	if failure.BalanceRef == "" {
		return engine.Posting{}, false
	}

	if failure.BalanceRef != posting.BalanceRef && !isOverdraftCompanion(request, posting.BalanceRef, failure.BalanceRef) {
		return engine.Posting{}, false
	}

	return posting, true
}

func isOverdraftCompanion(request engine.Request, postingRef, failureRef string) bool {
	var origin *engine.BalanceSnapshot

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
