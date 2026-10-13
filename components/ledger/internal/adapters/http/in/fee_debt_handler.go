// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"net/http"
	"time"

	"github.com/shopspring/decimal"

	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// FeeDebtView is one fee debt as Fees recorded it. Remaining is opened - settled - canceled
// + reopened over the changes recorded so far: a reopen recorded before the settlement it undoes
// briefly lifts it past opened, a settlement before the reopen it depends on takes it below zero.
type FeeDebtView struct {
	ID                  string             `json:"id" doc:"Origin transaction id and the fee debit posting the debt opened in" example:"01920000-0000-7000-8000-000000000001:from:1:debit"`
	OrganizationID      string             `json:"organizationId" example:"01920000-0000-7000-8000-00000000000a"`
	LedgerID            string             `json:"ledgerId" example:"01920000-0000-7000-8000-00000000000b"`
	DebtorBalance       string             `json:"debtorBalance" doc:"Debtor balance as alias#key" example:"@payer#default"`
	CreditBalance       string             `json:"creditBalance" doc:"Fee account balance the debt pays, as alias#key" example:"@fees#default"`
	OriginTransactionID string             `json:"originTransactionId" example:"01920000-0000-7000-8000-000000000001"`
	FeePackageID        string             `json:"feePackageId,omitempty" example:"01920000-0000-7000-8000-0000000000cc"`
	AssetCode           string             `json:"assetCode" example:"BRL"`
	Seq                 int64              `json:"seq" doc:"Settlement order among the debtor's debts, oldest first" example:"3"`
	OpenedAmount        decimal.Decimal    `json:"openedAmount" example:"70"`
	Remaining           decimal.Decimal    `json:"remaining" example:"40"`
	Entries             []FeeDebtEntryView `json:"entries" doc:"Recorded changes, oldest first"`
	OpenedAt            *time.Time         `json:"openedAt,omitempty"`
	CreatedAt           time.Time          `json:"createdAt" doc:"When the earliest recorded change was applied"`
	UpdatedAt           time.Time          `json:"updatedAt" doc:"When the latest recorded change was applied"`
}

// FeeDebtEntryView is one recorded change of a debt.
type FeeDebtEntryView struct {
	Kind          string          `json:"kind" enum:"opened,settled,canceled,reopened,refunded" example:"settled"`
	TransactionID string          `json:"transactionId" example:"01920000-0000-7000-8000-000000000002"`
	PostingRef    string          `json:"postingRef,omitempty" example:"to:0:credit:collect"`
	Amount        decimal.Decimal `json:"amount" example:"30"`
	AppliedAt     time.Time       `json:"appliedAt"`
}

// FeeDebtListBody is the cursor-paginated fee debt listing, oldest first.
type FeeDebtListBody struct {
	Items      []*FeeDebtView   `json:"items"`
	Limit      int              `json:"limit" example:"10"`
	NextCursor string           `json:"next_cursor,omitempty" example:"eyJpZCI6IjAxOTI..."`
	PrevCursor string           `json:"prev_cursor,omitempty" example:"eyJpZCI6IjAxOTE..."`
	OpenTotal  *decimal.Decimal `json:"openTotal,omitempty" doc:"What this debtor balance (account_alias and balance_key) owes over all its open debts, not only this page, each counting at most its opened amount; present only with account_alias" example:"25"`
}

// ListFeeDebtsV2Request binds the listing query; the core validates it.
type ListFeeDebtsV2Request struct {
	FeeV2Path

	AccountAlias string `query:"account_alias" doc:"Only the debts of this debtor account, in settlement order"`
	BalanceKey   string `query:"balance_key" doc:"Debtor balance key; requires account_alias (default \"default\")"`
	Status       string `query:"status" doc:"open: remaining above zero; settled: the rest. Absent lists both"`
	Limit        string `query:"limit" doc:"Number of items per page (default 10, max 100)"`
	Cursor       string `query:"cursor" doc:"Opaque cursor from a previous page"`
}

// ListFeeDebtsResponse carries the listing with 200 OK.
type ListFeeDebtsResponse struct {
	Status int
	Body   *FeeDebtListBody
}

// FeeDebtIDV2Request names one debt of the ledger.
type FeeDebtIDV2Request struct {
	FeeV2Path

	DebtID string `path:"fee_debt_id" doc:"Fee debt ID"`
}

// GetFeeDebtResponse carries the debt with 200 OK.
type GetFeeDebtResponse struct {
	Status int
	Body   *FeeDebtView
}

// ListFeeDebtsV2 lists the path ledger's fee debts oldest first.
func (handler *FeeDebtHandler) ListFeeDebtsV2(ctx context.Context, in *ListFeeDebtsV2Request) (*ListFeeDebtsResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	body, err := handler.listFeeDebts(ctx, orgID, ledgerID, in.AccountAlias, in.BalanceKey, in.Status, in.Limit, in.Cursor)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &ListFeeDebtsResponse{Status: http.StatusOK, Body: body}, nil
}

// GetFeeDebtV2 returns one fee debt of the path ledger.
func (handler *FeeDebtHandler) GetFeeDebtV2(ctx context.Context, in *FeeDebtIDV2Request) (*GetFeeDebtResponse, error) {
	orgID, ledgerID, err := parseFeeV2Path(in.FeeV2Path)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	view, err := handler.getFeeDebt(ctx, orgID, ledgerID, in.DebtID)
	if err != nil {
		return nil, pkgHTTP.HumaProblem(err)
	}

	return &GetFeeDebtResponse{Status: http.StatusOK, Body: view}, nil
}
