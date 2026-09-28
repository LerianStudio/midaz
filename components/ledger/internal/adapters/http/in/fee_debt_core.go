// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/url"
	"strconv"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/fee_debt"
	services "github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

const feeDebtMaxLimit = 100

// FeeDebtHandler serves the read-only Fees record of fee debts.
type FeeDebtHandler struct {
	Service *services.FeeDebtService
}

// listFeeDebts validates the listing query and pages the ledger's debts oldest first.
// balanceKey narrows accountAlias and defaults to the default key.
func (handler *FeeDebtHandler) listFeeDebts(ctx context.Context, organizationID, ledgerID uuid.UUID, accountAlias, balanceKey, limit, cursor string) (*FeeDebtListBody, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.list_fee_debts")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	query := fee_debt.ListQuery{Limit: 10, Cursor: cursor}

	if limit != "" {
		parsed, err := strconv.Atoi(limit)
		if err != nil || parsed < 1 {
			return nil, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityFeeDebt, "limit")
		}

		if parsed > feeDebtMaxLimit {
			return nil, pkg.ValidateBusinessError(constant.ErrPaginationLimitExceeded, constant.EntityFeeDebt, feeDebtMaxLimit)
		}

		query.Limit = parsed
	}

	if balanceKey != "" && accountAlias == "" {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityFeeDebt, "balance_key")
	}

	if accountAlias != "" {
		query.DebtorBalanceRef = mtransaction.AliasKey(accountAlias, balanceKey)
	}

	debts, pagination, err := handler.Service.ListFeeDebts(ctx, organizationID, ledgerID, query)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to list fee debts", err)

		return nil, err
	}

	body := &FeeDebtListBody{Items: make([]*FeeDebtView, 0, len(debts)), Limit: query.Limit, NextCursor: pagination.Next, PrevCursor: pagination.Prev}

	for _, debt := range debts {
		view, err := newFeeDebtView(debt)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to render fee debt", err)

			return nil, err
		}

		body.Items = append(body.Items, view)
	}

	return body, nil
}

// getFeeDebt returns one debt of the ledger. rawID is the path segment as received:
// a debt id carries colons, which clients may send percent-encoded.
func (handler *FeeDebtHandler) getFeeDebt(ctx context.Context, organizationID, ledgerID uuid.UUID, rawID string) (*FeeDebtView, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_fee_debt")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	id, err := url.PathUnescape(rawID)
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityFeeDebt, "debt_id")
	}

	span.SetAttributes(attribute.String("app.request.fee_debt_id", id))

	debt, err := handler.Service.GetFeeDebt(ctx, organizationID, ledgerID, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to get fee debt", err)

		return nil, err
	}

	view, err := newFeeDebtView(debt)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to render fee debt", err)

		return nil, err
	}

	return view, nil
}

func newFeeDebtView(debt *model.FeeDebt) (*FeeDebtView, error) {
	opened, err := decimal.NewFromString(debt.OpenedAmount.String())
	if err != nil {
		return nil, err
	}

	remaining, err := decimal.NewFromString(debt.Remaining.String())
	if err != nil {
		return nil, err
	}

	view := &FeeDebtView{
		ID:                  debt.ID,
		OrganizationID:      debt.OrganizationID,
		LedgerID:            debt.LedgerID,
		DebtorBalance:       debt.DebtorBalanceRef,
		CreditBalance:       debt.CreditBalanceRef,
		OriginTransactionID: debt.OriginTransactionID,
		FeePackageID:        debt.FeePackageID,
		AssetCode:           debt.AssetCode,
		Seq:                 debt.Seq,
		OpenedAmount:        opened,
		Remaining:           remaining,
		Entries:             make([]FeeDebtEntryView, len(debt.Entries)),
		CreatedAt:           debt.CreatedAt,
		UpdatedAt:           debt.UpdatedAt,
	}

	if !debt.OpenedAt.IsZero() {
		view.OpenedAt = &debt.OpenedAt
	}

	for i, entry := range debt.Entries {
		view.Entries[i] = FeeDebtEntryView{
			Kind: entry.Kind, TransactionID: entry.TransactionID, PostingRef: entry.PostingRef,
			Amount: entry.Amount, AppliedAt: entry.AppliedAt,
		}
	}

	return view, nil
}
