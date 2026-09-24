// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// AdmitPrepared is the producer boundary after fee composition and accounting
// preparation. Participation gates precede even the logical-entry projection.
func (c *ContextTracerCoordinator) AdmitPrepared(ctx context.Context, input ContextTracerInput, transaction mtransaction.Transaction, validated *mtransaction.Responses, balances []*mmodel.Balance) (ContextTracerAttempt, error) {
	if input.HonoredSkip || input.Settings.Mode == "" || input.Settings.Mode == "off" {
		return ContextTracerAttempt{Key: input.Key, Skipped: true}, nil
	}

	entries, err := tracerPreparedEntries(ctx, transaction, validated, balances, c.config.Facts.Bounds.MaxEntries)
	if err != nil {
		return ContextTracerAttempt{Key: input.Key}, err
	}

	input.Entries = entries

	return c.Admit(ctx, input)
}

// tracerPreparedEntries projects logical fee-inclusive legs, not resulting
// balance movements. It preserves repeated legs and pending destinations while
// excluding auxiliary overdraft movements. Call only after the off/skip gates.
func tracerPreparedEntries(ctx context.Context, input mtransaction.Transaction, validated *mtransaction.Responses, balances []*mmodel.Balance, maxEntries int) ([]tracer.PreparedEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sources, destinations := input.Send.Source.From, input.Send.Distribute.To
	if validated == nil || maxEntries <= 0 || len(sources) > maxEntries || len(destinations) > maxEntries-len(sources) || len(sources)+len(destinations) == 0 {
		return nil, constant.ErrTracerFactsUnavailable
	}

	indexed, err := indexTranslationBalances(balances)
	if err != nil {
		return nil, constant.ErrTracerFactsUnavailable
	}

	entries := make([]tracer.PreparedEntry, 0, len(sources)+len(destinations))
	for _, side := range []struct {
		legs      []mtransaction.FromTo
		amounts   map[string]mtransaction.Amount
		direction tracercontract.Direction
	}{
		{sources, validated.From, tracercontract.Debit}, {destinations, validated.To, tracercontract.Credit},
	} {
		for _, leg := range side.legs {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			amount, ok := side.amounts[leg.AccountAlias]

			balance := indexed[mtransaction.SplitAliasWithKey(leg.AccountAlias)]

			if !ok {
				return nil, constant.ErrTracerFactsUnavailable
			}

			entry, err := tracerPreparedEntry(balance, amount, input.Send.Asset, side.direction)
			if err != nil {
				return nil, err
			}

			entries = append(entries, entry)
		}
	}

	return entries, nil
}

func tracerPreparedEntry(balance *mmodel.Balance, amount mtransaction.Amount, asset string, direction tracercontract.Direction) (tracer.PreparedEntry, error) {
	if !amount.Value.IsPositive() || balance == nil || balance.Key == constant.OverdraftBalanceKey || balance.AssetCode != asset || (amount.Asset != "" && amount.Asset != balance.AssetCode) {
		return tracer.PreparedEntry{}, constant.ErrTracerFactsUnavailable
	}

	entry := tracer.PreparedEntry{External: balance.AccountType == constant.ExternalAccountType, Direction: direction, Amount: amount.Value, AssetCode: balance.AssetCode}
	if !entry.External {
		id, err := uuid.Parse(balance.AccountID)
		if err != nil || id == uuid.Nil {
			return tracer.PreparedEntry{}, constant.ErrTracerFactsUnavailable
		}

		entry.AccountID = id
	}

	return entry, nil
}
