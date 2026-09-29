// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/shopspring/decimal"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// completedTransactionFeeDebt is the executed amount (the send less the debt opened) and the
// frozen metadata plus the fee-debt keys the result derives, JSON array strings in result order.
// A revert drops the keys it inherited; a result without fee debt keeps the frozen map.
func completedTransactionFeeDebt(payload TransactionCompletionPlan, changes []accounting.FeeDebtChange) (decimal.Decimal, map[string]any, error) {
	amount := payload.TransactionInput.Send.Value

	var (
		openings    []FeeDebtOpening
		settlements []FeeDebtSettlement
	)

	for _, change := range changes {
		switch change.Kind {
		case accounting.FeeDebtOpened:
			amount = amount.Sub(change.Amount)
			openings = append(openings, FeeDebtOpening{DebtID: change.DebtID, DebtorRef: change.DebtorRef, CreditRef: change.CreditRef, Opened: change.Opened, Seq: change.Seq})
		case accounting.FeeDebtSettled:
			settlements = append(settlements, FeeDebtSettlement{
				DebtID: change.DebtID, DebtorRef: change.DebtorRef, CreditRef: change.CreditRef, Amount: change.Amount, Opened: change.Opened, Seq: change.Seq,
			})
		}
	}

	if payload.Action != constant.ActionRevert && openings == nil && settlements == nil {
		return amount, payload.TransactionInput.Metadata, nil
	}

	metadata := maps.Clone(payload.TransactionInput.Metadata)
	delete(metadata, constant.MetadataKeyFeeDebtOpenings)
	delete(metadata, constant.MetadataKeyFeeDebtSettlements)

	if metadata == nil && (openings != nil || settlements != nil) {
		metadata = make(map[string]any, 2)
	}

	if err := putFeeDebtMetadata(metadata, constant.MetadataKeyFeeDebtOpenings, openings); err != nil {
		return decimal.Decimal{}, nil, err
	}

	if err := putFeeDebtMetadata(metadata, constant.MetadataKeyFeeDebtSettlements, settlements); err != nil {
		return decimal.Decimal{}, nil, err
	}

	return amount, metadata, nil
}

func putFeeDebtMetadata[T any](metadata map[string]any, key string, entries []T) error {
	if entries == nil {
		return nil
	}

	encoded, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("encode %s metadata: %w", key, err)
	}

	metadata[key] = string(encoded)

	return nil
}

// feeDebtRecord is the Fees projection of a result's fee-debt changes; nil when it has none.
func feeDebtRecord(plan TransactionCompletionPlan, result accounting.ExecutionResult, metadata map[string]any) *FeeDebtRecord {
	if len(result.FeeDebt) == 0 {
		return nil
	}

	record := &FeeDebtRecord{OrganizationID: plan.OrganizationID, LedgerID: plan.LedgerID, Changes: slices.Clone(result.FeeDebt)}
	record.FeePackageID, _ = metadata["packageAppliedID"].(string)

	if result.AppliedAtUnixMicro > 0 {
		record.AppliedAt = time.UnixMicro(result.AppliedAtUnixMicro).UTC()
	}

	return record
}

func (service *TransactionCompletionService) recordFeeDebts(ctx context.Context, prepared []preparedTransactionCompletion) error {
	if service.feeDebt == nil {
		return nil
	}

	for _, unit := range prepared {
		if unit.feeDebt == nil {
			continue
		}

		if err := service.feeDebt.Apply(ctx, *unit.feeDebt); err != nil {
			return fmt.Errorf("record fee debts: %w", err)
		}
	}

	return nil
}

// feeDebtMetadataKeys are the reserved keys completion writes onto a stored document that lacks
// them, as a commit does on the transaction it shares with its pending.
var feeDebtMetadataKeys = [...]string{constant.MetadataKeyFeeDebtOpenings, constant.MetadataKeyFeeDebtSettlements}

// missingFeeDebtMetadata holds each fee-debt key the frozen document holds and the stored one
// lacks, or nil when none is missing.
func missingFeeDebtMetadata(expected, actual mongodb.JSON) map[string]any {
	var missing map[string]any

	for _, key := range feeDebtMetadataKeys {
		value, frozen := expected[key]
		if _, stored := actual[key]; frozen && !stored {
			if missing == nil {
				missing = make(map[string]any, len(feeDebtMetadataKeys))
			}

			missing[key] = value
		}
	}

	return missing
}

// postingAnchorRole is the role a posting's context holds on the posting's own balance: a collect
// or refund anchors on its debtor balance, every other posting on its primary.
func postingAnchorRole(postingType accounting.PostingType) string {
	switch postingType {
	case accounting.PostingCollect:
		return accounting.RoleFeeDebtDebit
	case accounting.PostingRefund:
		return accounting.RoleFeeDebtRefundCredit
	default:
		return accounting.RolePrimary
	}
}
