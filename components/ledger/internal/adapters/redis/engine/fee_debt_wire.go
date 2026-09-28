// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"fmt"
	"strconv"

	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// wireFeeDebt declares one debtor's live fee-debt list; its key is in the last
// block of KEYS, at KeyIndex.
type wireFeeDebt struct {
	OrganizationID string `json:"organizationId"`
	LedgerID       string `json:"ledgerId"`
	BalanceRef     string `json:"balanceRef"`
	KeyIndex       int    `json:"keyIndex"`
}

type wireFeeDebtRefund struct {
	DebtID    string `json:"debtId"`
	CreditRef string `json:"creditRef"`
	Opened    string `json:"opened"`
	Seq       string `json:"seq"`
}

type wireFeeDebtReopen struct {
	DebtID    string `json:"debtId"`
	DebtorRef string `json:"debtorRef"`
	CreditRef string `json:"creditRef"`
	Amount    string `json:"amount"`
	Opened    string `json:"opened"`
	Seq       string `json:"seq"`
}

type feeDebtDeclaration struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	BalanceRef     string
}

func (d feeDebtDeclaration) scopedRef() string {
	return scopedBalanceRef(d.OrganizationID, d.LedgerID, d.BalanceRef)
}

// feeDebtDeclarations is the union of every transaction's FeeDebtRefs, once per
// scoped balance, in first-appearance order: the order of the fee-debt key block.
func feeDebtDeclarations(request accounting.Execution) []feeDebtDeclaration {
	var declarations []feeDebtDeclaration

	seen := make(map[string]bool)

	for _, transaction := range request.Transactions {
		organizationID, ledgerID, _ := effectiveTransactionScope(request, transaction)
		for _, ref := range transaction.FeeDebtRefs {
			declaration := feeDebtDeclaration{OrganizationID: organizationID, LedgerID: ledgerID, BalanceRef: ref}
			if !seen[declaration.scopedRef()] {
				seen[declaration.scopedRef()] = true
				declarations = append(declarations, declaration)
			}
		}
	}

	return declarations
}

// resolveFeeDebtKeys resolves each declared list behind the tenant prefix of the
// balance keys.
func resolveFeeDebtKeys(ctx context.Context, request accounting.Execution, resolved *resolvedExecutionKeys) error {
	for _, declaration := range feeDebtDeclarations(request) {
		key, err := tmvalkey.GetKeyContext(ctx, utils.FeeDebtInternalKey(declaration.OrganizationID, declaration.LedgerID, declaration.BalanceRef))
		if err != nil {
			return err
		}

		if resolved.FeeDebts == nil {
			resolved.FeeDebts = make(map[string]string)
		}

		resolved.FeeDebts[declaration.scopedRef()] = key
	}

	return nil
}

// appendFeeDebtKeys closes the key inventory with one live list per declared
// debtor and declares each at its 1-based key index.
func appendFeeDebtKeys(request accounting.Execution, resolved resolvedExecutionKeys, keys []string) ([]string, []wireFeeDebt, error) {
	declarations := feeDebtDeclarations(request)
	if len(resolved.FeeDebts) != len(declarations) {
		return nil, nil, fmt.Errorf("resolved accounting fee-debt inventory does not match transactions")
	}

	var prepared []wireFeeDebt

	for _, declaration := range declarations {
		key := resolved.FeeDebts[declaration.scopedRef()]
		if key == "" || !validLogicalReference(declaration.BalanceRef) {
			return nil, nil, fmt.Errorf("invalid accounting fee-debt declaration")
		}

		keys = append(keys, key)
		prepared = append(prepared, wireFeeDebt{
			OrganizationID: declaration.OrganizationID.String(), LedgerID: declaration.LedgerID.String(),
			BalanceRef: declaration.BalanceRef, KeyIndex: len(keys),
		})
	}

	return keys, prepared, nil
}

// prepareFeeDebtPostingFields encodes the fee-debt fields of one posting; the
// engine owns every pairing rule between them.
func prepareFeeDebtPostingFields(posting accounting.Posting, prepared *wirePosting, maxBytes int) error {
	prepared.DeferShortfall, prepared.FundedByRef, prepared.Items = posting.DeferShortfall, posting.FundedByRef, posting.Items

	for _, refund := range posting.Refunds {
		opened, err := boundedDecimal(refund.Opened, maxBytes)
		if err != nil {
			return err
		}

		prepared.Refunds = append(prepared.Refunds, wireFeeDebtRefund{
			DebtID: refund.DebtID, CreditRef: refund.CreditRef, Opened: opened, Seq: strconv.FormatInt(refund.Seq, 10),
		})
	}

	return nil
}

func prepareFeeDebtReopens(reopens []accounting.FeeDebtReopen, maxBytes int) ([]wireFeeDebtReopen, error) {
	var prepared []wireFeeDebtReopen

	for _, reopen := range reopens {
		amount, err := boundedDecimal(reopen.Amount, maxBytes)
		if err != nil {
			return nil, err
		}

		opened, err := boundedDecimal(reopen.Opened, maxBytes)
		if err != nil {
			return nil, err
		}

		prepared = append(prepared, wireFeeDebtReopen{
			DebtID: reopen.DebtID, DebtorRef: reopen.DebtorRef, CreditRef: reopen.CreditRef,
			Amount: amount, Opened: opened, Seq: strconv.FormatInt(reopen.Seq, 10),
		})
	}

	return prepared, nil
}
