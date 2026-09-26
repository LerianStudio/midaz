// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"cmp"
	"context"
	"slices"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// validateBankAccountUnique is the rule: it refuses banking details whose bank account another
// live instrument of the organization holds (see sameBankAccount). type is not part of the key;
// the account is found by its search token under every enabled key, so rows written before a
// rotation count.
func (uc *UseCase) validateBankAccountUnique(ctx context.Context, organizationID string, self uuid.UUID, bd *mmodel.BankingDetails) error {
	if bd == nil || bd.Account == nil || *bd.Account == "" {
		return nil
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.validate_instrument_bank_account")
	defer span.End()

	// Limit 0 is no limit: every live instrument holding this account is compared.
	holders, err := uc.InstrumentRepo.FindAll(ctx, organizationID, uuid.Nil, http.QueryHeader{InstrumentBankingDetailsAccount: bd.Account}, false)
	if err != nil {
		recordSpanError(span, "Failed to find instruments holding the bank account", err)

		return err
	}

	for _, other := range holders {
		if other.ID != nil && *other.ID == self {
			continue
		}

		if sameBankAccount(other.BankingDetails, bd) {
			return pkg.ValidateBusinessError(constant.ErrBankAccountAlreadyRegistered, constant.EntityInstrument)
		}
	}

	return nil
}

// validateBankAccountPatch applies a merge-patch's bankId, branch and account to the stored
// instrument and checks the result. A patch that sets none of them and removes neither bankId
// nor branch cannot create a duplicate, so it reads nothing.
func (uc *UseCase) validateBankAccountPatch(ctx context.Context, organizationID string, holderID, id uuid.UUID, patch *mmodel.BankingDetails, fieldsToRemove []string) error {
	patched := patch != nil && (patch.BankID != nil || patch.Branch != nil || patch.Account != nil)
	if !patched && !slices.Contains(fieldsToRemove, "bankingDetails.bankId") && !slices.Contains(fieldsToRemove, "bankingDetails.branch") {
		return nil
	}

	stored, err := uc.InstrumentRepo.Find(ctx, organizationID, holderID, id, false)
	if err != nil {
		return err
	}

	merged := mmodel.BankingDetails{}
	if stored.BankingDetails != nil {
		merged = *stored.BankingDetails
	}

	for _, field := range fieldsToRemove {
		switch field {
		case "bankingDetails":
			merged = mmodel.BankingDetails{}
		case "bankingDetails.bankId":
			merged.BankID = nil
		case "bankingDetails.branch":
			merged.Branch = nil
		case "bankingDetails.account":
			merged.Account = nil
		}
	}

	if patch != nil {
		merged.BankID = cmp.Or(patch.BankID, merged.BankID)
		merged.Branch = cmp.Or(patch.Branch, merged.Branch)
		merged.Account = cmp.Or(patch.Account, merged.Account)
	}

	return uc.validateBankAccountUnique(ctx, organizationID, id, &merged)
}

// sameBankAccount compares bankId trimmed and the account exactly. Branches match when either is
// empty or both are equal once trimmed, numeric ones without leading zeros ("1" is "0001").
// A holder whose account was since removed or emptied still carries the old token and never matches.
func sameBankAccount(stored, candidate *mmodel.BankingDetails) bool {
	if stored == nil || stored.Account == nil || *stored.Account != *candidate.Account ||
		trimmedOrEmpty(stored.BankID) != trimmedOrEmpty(candidate.BankID) {
		return false
	}

	a, b := canonicalBranch(stored.Branch), canonicalBranch(candidate.Branch)

	return a == "" || b == "" || a == b
}

func canonicalBranch(s *string) string {
	branch := trimmedOrEmpty(s)
	if branch == "" || strings.Trim(branch, "0123456789") != "" {
		return branch
	}

	return cmp.Or(strings.TrimLeft(branch, "0"), "0")
}

func trimmedOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return strings.TrimSpace(*s)
}
