// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"github.com/shopspring/decimal"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// crossLedgerTransactionScopes preserves the one-to-one correspondence between
// normalized transaction legs and their ledger scopes.
type crossLedgerTransactionScopes struct {
	from []atomicTransactionBatchLedgerRef
	to   []atomicTransactionBatchLedgerRef
}

// decomposedCrossLedgerPart is one balanced, single-ledger transaction. Parts
// stay in first-seen debit-ledger order followed by destination-only ledgers.
type decomposedCrossLedgerPart struct {
	ledgerRef   atomicTransactionBatchLedgerRef
	transaction mtransaction.Transaction
}

type crossLedgerPartBuilder struct {
	ledgerRef atomicTransactionBatchLedgerRef
	from      []mtransaction.FromTo
	to        []mtransaction.FromTo
	fromTotal decimal.Decimal
	toTotal   decimal.Decimal
}

// decomposeCrossLedgerTransaction resolves every value expression once, then
// closes each participating ledger with its own @external/<asset> bridge. A
// ledger present on both sides crosses only its net difference.
func decomposeCrossLedgerTransaction(
	transaction mtransaction.Transaction,
	scopes crossLedgerTransactionScopes,
) ([]decomposedCrossLedgerPart, error) {
	if len(scopes.from) != len(transaction.Send.Source.From) || len(scopes.to) != len(transaction.Send.Distribute.To) {
		return nil, pkg.ValidateBusinessError(constant.ErrTransactionScopeMismatch, constant.EntityTransaction)
	}

	from, fromTotal, err := resolveCrossLedgerLegs(transaction.Send.Source.From, transaction.Send.Asset, transaction.Send.Value)
	if err != nil {
		return nil, err
	}

	to, toTotal, err := resolveCrossLedgerLegs(transaction.Send.Distribute.To, transaction.Send.Asset, transaction.Send.Value)
	if err != nil {
		return nil, err
	}

	if !fromTotal.Equal(transaction.Send.Value) || !toTotal.Equal(transaction.Send.Value) {
		return nil, pkg.ValidateBusinessError(constant.ErrTransactionValueMismatch, constant.EntityTransaction)
	}

	builders := make([]crossLedgerPartBuilder, 0, len(scopes.from)+len(scopes.to))
	byLedger := make(map[atomicTransactionBatchLedgerRef]int, cap(builders))
	ensureBuilder := func(ref atomicTransactionBatchLedgerRef) int {
		if index, ok := byLedger[ref]; ok {
			return index
		}

		index := len(builders)
		byLedger[ref] = index
		builders = append(builders, crossLedgerPartBuilder{ledgerRef: ref})

		return index
	}

	for index, leg := range from {
		builder := &builders[ensureBuilder(scopes.from[index])]
		builder.from = append(builder.from, leg)
		builder.fromTotal = builder.fromTotal.Add(leg.Amount.Value)
	}

	for index, leg := range to {
		builder := &builders[ensureBuilder(scopes.to[index])]
		builder.to = append(builder.to, leg)
		builder.toTotal = builder.toTotal.Add(leg.Amount.Value)
	}

	parts := make([]decomposedCrossLedgerPart, 0, len(builders))
	for index := range builders {
		builder := &builders[index]
		partTotal := decimal.Max(builder.fromTotal, builder.toTotal)
		difference := builder.fromTotal.Sub(builder.toTotal)

		switch {
		case difference.IsPositive():
			builder.to = append(builder.to, crossLedgerBridgeLeg(transaction.Send.Asset, difference, false))
		case difference.IsNegative():
			builder.from = append(builder.from, crossLedgerBridgeLeg(transaction.Send.Asset, difference.Abs(), true))
		}

		part := transaction
		part.Send = mtransaction.Send{
			Asset: transaction.Send.Asset,
			Value: partTotal,
			Source: mtransaction.Source{
				From: builder.from,
			},
			Distribute: mtransaction.Distribute{
				To: builder.to,
			},
		}

		parts = append(parts, decomposedCrossLedgerPart{ledgerRef: builder.ledgerRef, transaction: part})
	}

	return parts, nil
}

func resolveCrossLedgerLegs(
	legs []mtransaction.FromTo,
	asset string,
	total decimal.Decimal,
) ([]mtransaction.FromTo, decimal.Decimal, error) {
	resolved := make([]mtransaction.FromTo, 0, len(legs))
	resolvedTotal := decimal.Zero
	remaining := total

	for _, leg := range legs {
		var value decimal.Decimal

		switch {
		case leg.Amount != nil:
			if leg.Amount.Asset != asset {
				return nil, decimal.Zero, pkg.ValidateBusinessError(constant.ErrCrossLedgerAssetMismatch, constant.EntityTransaction)
			}

			value = leg.Amount.Value
		case leg.Share != nil:
			percentageOfPercentage := leg.Share.PercentageOfPercentage
			if percentageOfPercentage == 0 {
				percentageOfPercentage = 100
			}

			value = total.
				Mul(decimal.NewFromInt(leg.Share.Percentage)).
				Div(decimal.NewFromInt(100)).
				Mul(decimal.NewFromInt(percentageOfPercentage)).
				Div(decimal.NewFromInt(100))
		case leg.Remaining != "":
			value = remaining
		default:
			return nil, decimal.Zero, pkg.ValidateBusinessError(constant.ErrTransactionValueMismatch, constant.EntityTransaction)
		}

		cloned := leg
		cloned.Amount = &mtransaction.Amount{Asset: asset, Value: value}
		cloned.Share = nil
		cloned.Remaining = ""
		resolved = append(resolved, cloned)
		resolvedTotal = resolvedTotal.Add(value)
		remaining = remaining.Sub(value)
	}

	return resolved, resolvedTotal, nil
}

func crossLedgerBridgeLeg(asset string, amount decimal.Decimal, isFrom bool) mtransaction.FromTo {
	return mtransaction.FromTo{
		AccountAlias: constant.DefaultExternalAccountAliasPrefix + asset,
		Amount:       &mtransaction.Amount{Asset: asset, Value: amount},
		IsFrom:       isFrom,
	}
}
