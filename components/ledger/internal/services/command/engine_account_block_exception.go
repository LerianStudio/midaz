// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func engineAccountBlockExceptionOutflow(action string) (accounting.PostingType, error) {
	switch action {
	case constant.ActionDirect, constant.ActionRevert:
		return accounting.PostingDebit, nil
	case constant.ActionCommit:
		return accounting.PostingUnreserve, nil
	default:
		return "", invalidEngineTranslation("account-block exception is not eligible for this action")
	}
}

func bindEngineAccountBlockException(action string, grant *mtransaction.AccountBlockExceptionGrant, postings []accounting.Posting, balances map[string]*mmodel.Balance) (*accounting.AccountBlockException, error) {
	if grant == nil {
		return nil, nil
	}

	outflow, err := engineAccountBlockExceptionOutflow(action)
	if err != nil {
		return nil, err
	}

	var primary *accounting.Posting

	for i := range postings {
		posting := &postings[i]
		if posting.Type != outflow {
			continue
		}

		balance, exists := balances[posting.BalanceRef]
		if !exists || balance == nil || balance.Key == constant.OverdraftBalanceKey || balance.Alias != grant.Alias {
			continue
		}

		if primary != nil {
			return nil, invalidEngineAccountBlockException()
		}

		primary = posting
	}

	if primary == nil {
		return nil, invalidEngineAccountBlockException()
	}

	return &accounting.AccountBlockException{
		ExceptionID:       grant.ID,
		Alias:             grant.Alias,
		Amount:            primary.Amount,
		PrimaryPostingRef: primary.Ref,
	}, nil
}

func invalidEngineAccountBlockException() error {
	return pkg.ValidateBusinessError(constant.ErrAccountBlockExceptionInvalid, constant.EntityTransaction)
}

func recordEngineAccountBlockExceptionPresented(span trace.Span, presented bool) {
	span.SetAttributes(attribute.Bool("app.request.account_block_exception_presented", presented))
}

func recordEngineAccountBlockExceptionBypass(span trace.Span, exception *accounting.AccountBlockException) {
	count := 0
	if exception != nil {
		count = 1
	}

	span.SetAttributes(attribute.Int("app.account_block_exception_bypassed_balances", count))
}
