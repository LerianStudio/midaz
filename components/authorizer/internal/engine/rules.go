// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	authorizerv1 "github.com/LerianStudio/midaz/v3/proto/authorizer/v1"
)

func validateBalanceRules(
	balance *Balance,
	op *authorizerv1.BalanceOperation,
	preAvailable int64,
	preOnHold int64,
	postAvailable int64,
	postOnHold int64,
) (bool, string, string) {
	if balance == nil || op == nil {
		return false, RejectionInternalError, "invalid balance operation"
	}

	if ok, code, message := validateSufficientFundsRule(balance, postAvailable, op.GetOperation()); !ok {
		return false, code, message
	}

	if ok, code, message := validateExternalAccountRule(balance, postAvailable, op.GetOperation()); !ok {
		return false, code, message
	}

	if !balance.IsExternal {
		if ok, code, message := validateOnHoldRule(preAvailable, preOnHold, postAvailable, postOnHold, op.GetOperation(), op.GetAmount()); !ok {
			return false, code, message
		}
	} else if postOnHold < 0 {
		return false, RejectionAmountExceedsHold, "on_hold cannot become negative"
	}

	if !balance.AllowSending && strings.EqualFold(op.GetOperation(), constant.DEBIT) {
		return false, RejectionAccountIneligible, "account is not allowed to send"
	}

	if !balance.AllowReceiving && strings.EqualFold(op.GetOperation(), constant.CREDIT) {
		return false, RejectionAccountIneligible, "account is not allowed to receive"
	}

	return true, "", ""
}

func validateSufficientFundsRule(balance *Balance, postAvailable int64, _ string) (bool, string, string) {
	if balance == nil {
		return false, RejectionInternalError, "balance is nil"
	}

	if balance.IsExternal {
		return true, "", ""
	}

	if postAvailable < 0 {
		return false, RejectionInsufficientFunds, "insufficient funds"
	}

	return true, "", ""
}

func validateExternalAccountRule(balance *Balance, postAvailable int64, operation string) (bool, string, string) {
	if balance == nil {
		return false, RejectionInternalError, "balance is nil"
	}

	if !balance.IsExternal {
		return true, "", ""
	}

	if strings.EqualFold(operation, constant.CREDIT) && postAvailable > 0 {
		return false, RejectionInsufficientFunds, "external account cannot become positive"
	}

	return true, "", ""
}

func validateOnHoldRule(
	preAvailable int64,
	preOnHold int64,
	postAvailable int64,
	postOnHold int64,
	operation string,
	rawAmount int64,
) (bool, string, string) {
	op := strings.ToUpper(operation)

	switch op {
	case constant.DEBIT, constant.ONHOLD:
		if preAvailable-preOnHold < 0 {
			return false, RejectionAmountExceedsHold, "available amount is below hold"
		}

		if preAvailable-postAvailable < 0 {
			return false, RejectionAmountExceedsHold, "invalid debit delta for hold validation"
		}

		if rawAmount > 0 && preAvailable-preOnHold < preAvailable-postAvailable {
			return false, RejectionAmountExceedsHold, fmt.Sprintf("amount exceeds available after hold (available=%d hold=%d)", preAvailable, preOnHold)
		}
	case constant.CREDIT, constant.RELEASE:
		if postOnHold < 0 {
			return false, RejectionAmountExceedsHold, "on_hold cannot become negative"
		}
	}

	return true, "", ""
}
