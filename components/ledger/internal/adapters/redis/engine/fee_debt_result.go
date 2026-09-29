// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
)

// feeDebtMovementTypes is the only movement type each fee-debt role records.
var feeDebtMovementTypes = map[string]accounting.PostingType{
	accounting.RoleFeeDebtDebit:        accounting.PostingDebit,
	accounting.RoleFeeDebtCredit:       accounting.PostingCredit,
	accounting.RoleFeeDebtRefundCredit: accounting.PostingCredit,
	accounting.RoleFeeDebtRefundDebit:  accounting.PostingDebit,
}

type resultFeeDebtChange struct {
	TransactionID       string                       `json:"transactionId"`
	PostingRef          string                       `json:"postingRef"`
	Kind                accounting.FeeDebtChangeKind `json:"kind"`
	DebtID              string                       `json:"debtId"`
	DebtorRef           string                       `json:"debtorRef"`
	CreditRef           string                       `json:"creditRef"`
	OriginTransactionID string                       `json:"originTransactionId"`
	Seq                 string                       `json:"seq"`
	AssetCode           string                       `json:"assetCode"`
	Amount              string                       `json:"amount"`
	Opened              string                       `json:"opened"`
}

// movementOrdinal returns the canonical nonnegative ordinal that follows prefix
// in a movement reference.
func movementOrdinal(ref, prefix string) (int, bool) {
	text, found := strings.CutPrefix(ref, prefix)
	ordinal, err := strconv.Atoi(text)

	return ordinal, found && err == nil && ordinal >= 0 && strconv.Itoa(ordinal) == text
}

// feeDebtMovementSub correlates a fee-debt movement with its collect or refund
// posting and returns its sub-position: 0 on the debtor, 1 on the debtor's
// overdraft companion (refund only), 2+ordinal per item.
func feeDebtMovementSub(role string, ordinal int, posting accounting.Posting, source, target accounting.BalanceSnapshot) (int64, bool) {
	onDebtor := ordinal == 0 && target.BalanceRef == source.BalanceRef

	switch {
	case role == accounting.RoleFeeDebtDebit && posting.Type == accounting.PostingCollect:
		return 0, onDebtor
	case role == accounting.RoleFeeDebtRefundCredit && posting.Type == accounting.PostingRefund:
		return 0, onDebtor
	case role == accounting.RoleOverdraftCompanion && posting.Type == accounting.PostingRefund:
		return 1, ordinal == 0 && target.Key == "overdraft" && target.AccountID == source.AccountID && target.BalanceRef != source.BalanceRef
	case role == accounting.RoleFeeDebtCredit && posting.Type == accounting.PostingCollect:
		return int64(ordinal) + 2, ordinal < len(posting.Items) && target.BalanceRef != source.BalanceRef
	case role == accounting.RoleFeeDebtRefundDebit && posting.Type == accounting.PostingRefund:
		return int64(ordinal) + 2, ordinal < len(posting.Refunds) && target.BalanceRef == posting.Refunds[ordinal].CreditRef
	default:
		return 0, false
	}
}

// decodeFeeDebtResult decodes the result's fee-debt changes, pairs them with the
// movements already decoded and attaches them to the result.
func decodeFeeDebtResult(result *accounting.ExecutionResult, raw []json.RawMessage, request accounting.Execution) (*accounting.ExecutionResult, error) {
	changes, err := decodeFeeDebtChanges(raw, request)
	if err != nil {
		return nil, err
	}

	if err := validateFeeDebtResult(result.Movements, changes, request); err != nil {
		return nil, err
	}

	result.FeeDebt = changes

	return result, nil
}

func decodeFeeDebtChanges(raw []json.RawMessage, request accounting.Execution) ([]accounting.FeeDebtChange, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	changes := make([]accounting.FeeDebtChange, 0, len(raw))

	for _, item := range raw {
		var wire resultFeeDebtChange
		if err := decodeStrict(item, &wire); err != nil {
			return nil, err
		}

		transactionID, errTransaction := uuid.Parse(wire.TransactionID)
		origin, errOrigin := uuid.Parse(wire.OriginTransactionID)
		seq, errSeq := strictVersion(wire.Seq)
		amount, errAmount := strictDecimal(wire.Amount)
		opened, errOpened := strictDecimal(wire.Opened)
		_, _, known := transactionScopeForID(request, transactionID)

		if errors.Join(errTransaction, errOrigin, errSeq, errAmount, errOpened) != nil || !known || seq <= 0 ||
			!validFeeDebtKind(wire.Kind, wire.PostingRef) || wire.DebtID == "" || wire.AssetCode == "" ||
			!validLogicalReference(wire.DebtorRef) || !validLogicalReference(wire.CreditRef) || !amount.IsPositive() || amount.GreaterThan(opened) {
			return nil, errors.New("invalid accounting fee-debt change")
		}

		changes = append(changes, accounting.FeeDebtChange{
			TransactionID: transactionID, PostingRef: wire.PostingRef, Kind: wire.Kind, DebtID: wire.DebtID,
			DebtorRef: wire.DebtorRef, CreditRef: wire.CreditRef, OriginTransactionID: origin, Seq: seq,
			AssetCode: wire.AssetCode, Amount: amount, Opened: opened,
		})
	}

	return changes, nil
}

// validFeeDebtKind accepts the five kinds; only canceled and reopened carry no posting.
func validFeeDebtKind(kind accounting.FeeDebtChangeKind, postingRef string) bool {
	switch kind {
	case accounting.FeeDebtOpened, accounting.FeeDebtSettled, accounting.FeeDebtRefunded:
		return postingRef != ""
	case accounting.FeeDebtCanceled, accounting.FeeDebtReopened:
		return postingRef == ""
	default:
		return false
	}
}

// validFeeDebtMovement accepts a movement whose fee-debt role records its one
// type and whose overdraft delta is its overdraft change: zero, except that a
// refund's debtor credit repays; any other role is not its concern.
func validFeeDebtMovement(movement accounting.Movement) bool {
	expected, feeDebt := feeDebtMovementTypes[movement.Role]
	delta := movement.After.OverdraftUsed.Sub(movement.Before.OverdraftUsed)

	return !feeDebt || (movement.Type == expected && movement.OverdraftDelta.Equal(delta) &&
		(delta.IsZero() || (movement.Role == accounting.RoleFeeDebtRefundCredit && delta.IsNegative())))
}

// validateFeeDebtResult pairs every fee-debt movement with its change and every
// change with a movement: deferrals add up, collect credits are its settlements,
// and refund debits are each entry's expected refund.
func validateFeeDebtResult(movements []accounting.Movement, changes []accounting.FeeDebtChange, request accounting.Execution) error {
	if len(changes) > 0 && len(movements) == 0 {
		return errors.New("accounting fee-debt changes without movements")
	}

	for _, transaction := range request.Transactions {
		moved := make(map[string][]accounting.Movement)

		for _, movement := range movements {
			if movement.TransactionID == transaction.ID {
				moved[movement.PostingRef+"\x00"+movement.Role] = append(moved[movement.PostingRef+"\x00"+movement.Role], movement)
			}
		}

		changed := make(map[string][]accounting.FeeDebtChange)

		for _, change := range changes {
			if change.TransactionID == transaction.ID {
				changed[change.PostingRef+"\x00"+string(change.Kind)] = append(changed[change.PostingRef+"\x00"+string(change.Kind)], change)
			}
		}

		for _, posting := range transaction.Postings {
			if err := validatePostingFeeDebt(transaction, posting, moved, changed); err != nil {
				return err
			}
		}
	}

	return nil
}

func validatePostingFeeDebt(transaction accounting.Transaction, posting accounting.Posting, moved map[string][]accounting.Movement, changed map[string][]accounting.FeeDebtChange) error {
	switch {
	case posting.DeferShortfall:
		primary, opened := moved[posting.Ref+"\x00"+accounting.RolePrimary], changed[posting.Ref+"\x00"+string(accounting.FeeDebtOpened)]
		paid := decimal.Zero

		for _, movement := range primary {
			paid = paid.Add(movement.Amount)
		}

		for _, change := range opened {
			paid = paid.Add(change.Amount)
			if len(opened) != 1 || change.DebtorRef != posting.BalanceRef || change.CreditRef != fundingCreditRef(transaction, posting.Ref) {
				return errors.New("invalid accounting fee-debt opening")
			}
		}

		if !paid.Equal(posting.Amount) {
			return errors.New("accounting deferrable debit does not add up")
		}
	case posting.Type == accounting.PostingCollect:
		return pairFeeDebtItems(posting, moved[posting.Ref+"\x00"+accounting.RoleFeeDebtDebit], moved[posting.Ref+"\x00"+accounting.RoleFeeDebtCredit],
			changed[posting.Ref+"\x00"+string(accounting.FeeDebtSettled)], func(ordinal int) (string, string, decimal.Decimal, bool) {
				return posting.Items[ordinal], "", decimal.Zero, false
			})
	case posting.Type == accounting.PostingRefund:
		return pairFeeDebtItems(posting, moved[posting.Ref+"\x00"+accounting.RoleFeeDebtRefundCredit], moved[posting.Ref+"\x00"+accounting.RoleFeeDebtRefundDebit],
			changed[posting.Ref+"\x00"+string(accounting.FeeDebtRefunded)], func(ordinal int) (string, string, decimal.Decimal, bool) {
				refund := posting.Refunds[ordinal]
				return refund.DebtID, refund.CreditRef, refund.ExpectedRefund, true
			})
	}

	return nil
}

// pairFeeDebtItems checks one collect or refund posting: at most one debtor
// movement for the total, overdraft repaid included, and one item movement per
// change, in order, naming the item at its ordinal, on its creditor, for its amount.
func pairFeeDebtItems(posting accounting.Posting, debtor, items []accounting.Movement, changes []accounting.FeeDebtChange, item func(int) (string, string, decimal.Decimal, bool)) error {
	if len(items) != len(changes) || (len(debtor) != 1 && len(items) > 0) || (len(debtor) != 0 && len(items) == 0) {
		return errors.New("accounting fee-debt movements do not pair with changes")
	}

	total := decimal.Zero

	for i, movement := range items {
		ordinal, _ := strconv.Atoi(movement.Ref[strings.LastIndexByte(movement.Ref, ':')+1:])
		debtID, creditRef, amount, exact := item(ordinal)
		change := changes[i]

		if change.DebtID != debtID || change.DebtorRef != posting.BalanceRef || change.CreditRef != movement.BalanceRef ||
			!change.Amount.Equal(movement.Amount) || (exact && (change.CreditRef != creditRef || !change.Amount.Equal(amount))) {
			return errors.New("accounting fee-debt movement does not match its change")
		}

		total = total.Add(movement.Amount)
	}

	if len(debtor) == 1 && (!debtor[0].Amount.Sub(debtor[0].OverdraftDelta).Equal(total) || total.GreaterThan(posting.Amount)) {
		return errors.New("accounting fee-debt total does not match its debtor movement")
	}

	return nil
}

func fundingCreditRef(transaction accounting.Transaction, debitRef string) string {
	for _, posting := range transaction.Postings {
		if posting.FundedByRef == debitRef {
			return posting.BalanceRef
		}
	}

	return ""
}
