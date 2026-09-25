// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"bytes"
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// AccountDebit is the gross consumption candidate for an internal account and
// asset. Tracer computes it from facts; callers cannot supply precomputed usage.
// It is not yet a reservation: limits, policy and atomic persistence still apply.
type AccountDebit struct {
	AccountID uuid.UUID
	Asset     tracercontract.AssetRef
	Amount    decimal.Decimal
}

// AccountDebits computes gross internal debits, including each fee payer's
// entries. Credits do not offset debits; external participants produce no account
// counters. A reversal's prepared entries are treated as new debits, with no
// automatic refund of previously committed consumption.
//
// Results are ordered by account ID for deterministic downstream processing.
// This order alone does not replace consistent ordering of database locks,
// shared counters and audit resources in the reservation orchestrator.
func AccountDebits(ctx context.Context, input tracercontract.Context, namespace string, limits tracercontract.Limits) ([]AccountDebit, error) {
	if err := input.Validate(ctx, namespace, limits); err != nil {
		return nil, err
	}

	// Context validation guarantees one official asset per internal account.
	// Different asset identities are never netted or converted, even when their
	// human-readable codes are equal.
	byAccount := make(map[uuid.UUID]AccountDebit, len(input.Accounts))

	for _, entry := range input.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if entry.External || entry.Direction != tracercontract.Debit {
			continue
		}

		amount, err := entry.Amount.Decimal(ctx, limits)
		if err != nil {
			return nil, err
		}

		total := byAccount[entry.AccountID]
		total.AccountID = entry.AccountID
		total.Asset = entry.Asset
		total.Amount = total.Amount.Add(amount)

		// Bound aggregate carries as well as individual quantities; never truncate
		// to fit a counter or let a valid set of entries bypass resource limits.
		if _, err := tracercontract.AmountFromDecimal(ctx, total.Amount, limits); err != nil {
			return nil, err
		}

		byAccount[entry.AccountID] = total
	}

	result := make([]AccountDebit, 0, len(byAccount))
	for _, total := range byAccount {
		result = append(result, total)
	}

	slices.SortFunc(result, func(a, b AccountDebit) int {
		return bytes.Compare(a.AccountID[:], b.AccountID[:])
	})

	return result, nil
}
