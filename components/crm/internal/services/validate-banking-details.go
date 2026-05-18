// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

func validateCompleteBankingIdentity(ctx context.Context, bankingDetails *mmodel.BankingDetails) error {
	if bankingDetails == nil {
		return nil
	}

	hasAnyIdentityField := nonEmptyStringPtr(bankingDetails.BankID) ||
		nonEmptyStringPtr(bankingDetails.Branch) ||
		nonEmptyStringPtr(bankingDetails.Account) ||
		nonEmptyStringPtr(bankingDetails.Type)
	if !hasAnyIdentityField {
		return nil
	}

	missingFields := make([]string, 0, 4)
	if !nonEmptyStringPtr(bankingDetails.BankID) {
		missingFields = append(missingFields, "bankingDetails.bankId")
	}

	if !nonEmptyStringPtr(bankingDetails.Branch) {
		missingFields = append(missingFields, "bankingDetails.branch")
	}

	if !nonEmptyStringPtr(bankingDetails.Account) {
		missingFields = append(missingFields, "bankingDetails.account")
	}

	if !nonEmptyStringPtr(bankingDetails.Type) {
		missingFields = append(missingFields, "bankingDetails.type")
	}

	if len(missingFields) == 0 {
		return nil
	}

	return missingBankingIdentityFieldsError(ctx, missingFields)
}

func validateBankingIdentityPatch(ctx context.Context, bankingDetails *mmodel.BankingDetails, fieldsToRemove []string) error {
	if removesBankingIdentityField(fieldsToRemove) {
		return missingBankingIdentityFieldsError(ctx, []string{"bankingDetails.bankId", "bankingDetails.branch", "bankingDetails.account", "bankingDetails.type"})
	}

	return validateCompleteBankingIdentity(ctx, bankingDetails)
}

func missingBankingIdentityFieldsError(ctx context.Context, missingFields []string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "service.validate_complete_banking_identity")
	defer span.End()

	err := pkg.ValidateBusinessError(cn.ErrMissingFieldsInRequest, cn.EntityAlias, strings.Join(missingFields, ", "))
	libOpenTelemetry.HandleSpanBusinessErrorEvent(span, "Incomplete banking identity", err)
	logger.Log(ctx, libLog.LevelWarn, "Incomplete banking identity", libLog.Err(err))

	return err
}

func removesBankingIdentityField(fieldsToRemove []string) bool {
	for _, field := range fieldsToRemove {
		switch strings.TrimSpace(field) {
		case "bankingDetails.bankId", "bankingDetails.branch", "bankingDetails.account", "bankingDetails.type",
			"banking_details.bank_id", "banking_details.branch", "banking_details.account", "banking_details.type":
			return true
		}
	}

	return false
}

func nonEmptyStringPtr(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}
