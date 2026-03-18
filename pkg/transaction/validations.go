// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"strconv"
	"strings"

	"github.com/LerianStudio/lib-commons/v4/commons"
	constant "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	"github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	pkgConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
)

// ValidateBalancesRules function with some validates in accounts operations
func ValidateBalancesRules(ctx context.Context, transaction Transaction, validate Responses, balances []*Balance) error {
	logger, tracer, _, _ := commons.NewTrackingFromContext(ctx)

	_, spanValidateBalances := tracer.Start(ctx, "validations.validate_balances_rules")
	defer spanValidateBalances.End()

	if len(balances) != (len(validate.From) + len(validate.To)) {
		err := commons.ValidateBusinessError(constant.ErrAccountIneligibility, "ValidateAccounts")

		opentelemetry.HandleSpanBusinessErrorEvent(spanValidateBalances, "validations.validate_balances_rules", err)

		return err
	}

	for _, balance := range balances {
		if err := validateFromBalances(balance, validate.From, validate.Asset, validate.Pending); err != nil {
			opentelemetry.HandleSpanBusinessErrorEvent(spanValidateBalances, "validations.validate_from_balances_", err)

			logger.Log(ctx, libLog.LevelError, "validations.validate_from_balances_err", libLog.Err(err))

			return err
		}

		if err := validateToBalances(balance, validate.To, validate.Asset); err != nil {
			opentelemetry.HandleSpanBusinessErrorEvent(spanValidateBalances, "validations.validate_to_balances_", err)

			logger.Log(ctx, libLog.LevelError, "validations.validate_to_balances_err", libLog.Err(err))

			return err
		}
	}

	return nil
}

func validateFromBalances(balance *Balance, from map[string]Amount, asset string, pending bool) error {
	for key := range from {
		balanceAliasKey := AliasKey(balance.Alias, balance.Key)
		if key == balance.ID || SplitAliasWithKey(key) == balanceAliasKey {
			if balance.AssetCode != asset {
				return commons.ValidateBusinessError(constant.ErrAssetCodeNotFound, "validateFromAccounts")
			}

			if !balance.AllowSending {
				return commons.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateFromAccounts")
			}

			if pending && balance.AccountType == constant.ExternalAccountType {
				return commons.ValidateBusinessError(constant.ErrOnHoldExternalAccount, "validateBalance", balance.Alias)
			}
		}
	}

	return nil
}

func validateToBalances(balance *Balance, to map[string]Amount, asset string) error {
	balanceAliasKey := AliasKey(balance.Alias, balance.Key)
	for key := range to {
		if key == balance.ID || SplitAliasWithKey(key) == balanceAliasKey {
			if balance.AssetCode != asset {
				return commons.ValidateBusinessError(constant.ErrAssetCodeNotFound, "validateToAccounts")
			}

			if !balance.AllowReceiving {
				return commons.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateToAccounts")
			}
		}
	}

	return nil
}

// ValidateFromToOperation func that validate operate balance
func ValidateFromToOperation(ft FromTo, validate Responses, balance *Balance) (Amount, Balance, error) {
	if ft.IsFrom {
		ba, err := OperateBalances(validate.From[ft.AccountAlias], *balance)
		if err != nil {
			return Amount{}, Balance{}, err
		}

		return validate.From[ft.AccountAlias], ba, nil
	} else {
		ba, err := OperateBalances(validate.To[ft.AccountAlias], *balance)
		if err != nil {
			return Amount{}, Balance{}, err
		}

		return validate.To[ft.AccountAlias], ba, nil
	}
}

// AliasKey function to concatenate alias with balance key
func AliasKey(alias, balanceKey string) string {
	if balanceKey == "" {
		balanceKey = "default"
	}

	return alias + "#" + balanceKey
}

// SplitAlias function to split alias with index
func SplitAlias(alias string) string {
	if strings.Contains(alias, "#") {
		return strings.Split(alias, "#")[1]
	}

	return alias
}

// ConcatAlias function to concat alias with index
func ConcatAlias(i int, alias string) string {
	return strconv.Itoa(i) + "#" + alias
}

// OperateBalances Function to sum or sub two balances and Normalize the scale
func OperateBalances(amount Amount, balance Balance) (Balance, error) {
	available, onHold, matched := applyBalanceChange(amount, balance)
	if !matched {
		return balance, nil
	}

	return Balance{
		Available: available,
		OnHold:    onHold,
		Version:   balance.Version + 1,
	}, nil
}

// applyBalanceChange computes the new Available and OnHold values for a given
// operation+transaction type combination. Returns matched=false for unknown operations.
func applyBalanceChange(amount Amount, balance Balance) (available, onHold decimal.Decimal, matched bool) {
	switch amount.TransactionType {
	case constant.PENDING:
		return applyPendingBalance(amount, balance)
	case constant.CANCELED:
		return applyCanceledBalance(amount, balance)
	case constant.APPROVED:
		return applyApprovedBalance(amount, balance)
	case constant.CREATED:
		return applyCreatedBalance(amount, balance)
	default:
		return balance.Available, balance.OnHold, false
	}
}

func applyPendingBalance(amount Amount, balance Balance) (available, onHold decimal.Decimal, matched bool) {
	switch {
	case amount.Operation == constant.DEBIT && amount.RouteValidationEnabled:
		// Double-entry: DEBIT only decrements Available.
		return balance.Available.Sub(amount.Value), balance.OnHold, true
	case amount.Operation == constant.ONHOLD && amount.RouteValidationEnabled:
		// Double-entry: ON_HOLD only increments OnHold.
		return balance.Available, balance.OnHold.Add(amount.Value), true
	case amount.Operation == constant.ONHOLD:
		// Legacy: ON_HOLD moves from Available to OnHold.
		return balance.Available.Sub(amount.Value), balance.OnHold.Add(amount.Value), true
	default:
		return balance.Available, balance.OnHold, false
	}
}

func applyCanceledBalance(amount Amount, balance Balance) (available, onHold decimal.Decimal, matched bool) {
	switch {
	case amount.Operation == constant.RELEASE && amount.RouteValidationEnabled:
		// Double-entry: RELEASE only decrements OnHold.
		return balance.Available, balance.OnHold.Sub(amount.Value), true
	case amount.Operation == constant.RELEASE:
		// Legacy: RELEASE moves from OnHold to Available.
		return balance.Available.Add(amount.Value), balance.OnHold.Sub(amount.Value), true
	case amount.Operation == constant.CREDIT && amount.RouteValidationEnabled:
		// Double-entry: CREDIT only increments Available.
		return balance.Available.Add(amount.Value), balance.OnHold, true
	default:
		return balance.Available, balance.OnHold, false
	}
}

func applyApprovedBalance(amount Amount, balance Balance) (available, onHold decimal.Decimal, matched bool) {
	switch amount.Operation {
	case constant.DEBIT:
		return balance.Available, balance.OnHold.Sub(amount.Value), true
	case constant.ONHOLD:
		// Route validation: ON_HOLD in APPROVED decrements OnHold (same as DEBIT).
		return balance.Available, balance.OnHold.Sub(amount.Value), true
	case constant.CREDIT:
		return balance.Available.Add(amount.Value), balance.OnHold, true
	default:
		return balance.Available, balance.OnHold, false
	}
}

func applyCreatedBalance(amount Amount, balance Balance) (available, onHold decimal.Decimal, matched bool) {
	switch amount.Operation {
	case constant.DEBIT:
		return balance.Available.Sub(amount.Value), balance.OnHold, true
	case constant.CREDIT:
		return balance.Available.Add(amount.Value), balance.OnHold, true
	default:
		return balance.Available, balance.OnHold, false
	}
}

// IsDoubleEntrySource returns true when an Amount entry requires double-entry
// splitting (two separate operations that each affect a single balance field).
// This applies to source entries with route validation enabled for PENDING or CANCELED transactions.
func IsDoubleEntrySource(amt Amount) bool {
	if !amt.RouteValidationEnabled {
		return false
	}

	switch amt.TransactionType {
	case constant.PENDING:
		return amt.Operation == constant.ONHOLD
	case constant.CANCELED:
		return amt.Operation == constant.RELEASE
	default:
		return false
	}
}

// SplitDoubleEntryOps takes an Amount that qualifies for double-entry (per IsDoubleEntrySource)
// and returns the two split operations. For PENDING: DEBIT + ONHOLD. For CANCELED: RELEASE + CREDIT.
// The caller must check IsDoubleEntrySource first; behavior is undefined for non-qualifying amounts.
func SplitDoubleEntryOps(amt Amount) (Amount, Amount) {
	op1 := amt
	op2 := amt

	switch amt.TransactionType {
	case constant.PENDING:
		op1.Operation = constant.DEBIT
		op1.Direction = pkgConstant.DirectionDebit
		op2.Operation = constant.ONHOLD
		op2.Direction = pkgConstant.DirectionCredit
	case constant.CANCELED:
		op1.Operation = constant.RELEASE
		op1.Direction = pkgConstant.DirectionDebit
		op2.Operation = constant.CREDIT
		op2.Direction = pkgConstant.DirectionCredit
	}

	return op1, op2
}

// DetermineOperation determines the operation type and direction for a balance entry.
// Returns (type, direction) where type is the operation type (DEBIT, CREDIT, ON_HOLD, RELEASE)
// and direction is the accounting direction ("debit" or "credit").
func DetermineOperation(isPending bool, isFrom bool, transactionType string) (string, string) {
	switch {
	case isPending && transactionType == constant.PENDING:
		if isFrom {
			return constant.ONHOLD, pkgConstant.DirectionDebit
		}

		return constant.CREDIT, pkgConstant.DirectionCredit
	case isPending && isFrom && transactionType == constant.CANCELED:
		return constant.RELEASE, pkgConstant.DirectionCredit
	case isPending && transactionType == constant.APPROVED:
		if isFrom {
			return constant.DEBIT, pkgConstant.DirectionDebit
		}

		return constant.CREDIT, pkgConstant.DirectionCredit
	case !isPending:
		if isFrom {
			return constant.DEBIT, pkgConstant.DirectionDebit
		}

		return constant.CREDIT, pkgConstant.DirectionCredit
	default:
		return constant.CREDIT, pkgConstant.DirectionCredit
	}
}

// CalculateTotal Calculate total for sources/destinations based on shares, amounts and remains
func CalculateTotal(fromTos []FromTo, transaction Transaction, transactionType string, t chan decimal.Decimal, ft chan map[string]Amount, sd chan []string, or chan map[string]string) {
	fmto := make(map[string]Amount)
	scdt := make([]string, 0, len(fromTos))

	total := decimal.NewFromInt(0)

	remaining := Amount{
		Asset:           transaction.Send.Asset,
		Value:           transaction.Send.Value,
		TransactionType: transactionType,
	}

	operationRoute := make(map[string]string)

	for i := range fromTos {
		if fromTos[i].RouteID != nil {
			operationRoute[fromTos[i].AccountAlias] = *fromTos[i].RouteID
		}

		operation, direction := DetermineOperation(transaction.Pending, fromTos[i].IsFrom, transactionType)

		if fromTos[i].Share != nil && fromTos[i].Share.Percentage != 0 {
			oneHundred := decimal.NewFromInt(100)

			percentage := decimal.NewFromInt(fromTos[i].Share.Percentage)

			percentageOfPercentage := decimal.NewFromInt(fromTos[i].Share.PercentageOfPercentage)
			if percentageOfPercentage.IsZero() {
				percentageOfPercentage = oneHundred
			}

			firstPart := percentage.Div(oneHundred)
			secondPart := percentageOfPercentage.Div(oneHundred)
			shareValue := transaction.Send.Value.Mul(firstPart).Mul(secondPart)

			fmto[fromTos[i].AccountAlias] = Amount{
				Asset:           transaction.Send.Asset,
				Value:           shareValue,
				Operation:       operation,
				TransactionType: transactionType,
				Direction:       direction,
			}

			total = total.Add(shareValue)
			remaining.Value = remaining.Value.Sub(shareValue)
		}

		if fromTos[i].Amount != nil && fromTos[i].Amount.Value.IsPositive() {
			amount := Amount{
				Asset:           fromTos[i].Amount.Asset,
				Value:           fromTos[i].Amount.Value,
				Operation:       operation,
				TransactionType: transactionType,
				Direction:       direction,
			}

			fmto[fromTos[i].AccountAlias] = amount
			total = total.Add(amount.Value)

			remaining.Value = remaining.Value.Sub(amount.Value)
		}

		if !commons.IsNilOrEmpty(&fromTos[i].Remaining) {
			total = total.Add(remaining.Value)

			remaining.Operation = operation
			remaining.Direction = direction

			fmto[fromTos[i].AccountAlias] = remaining
			fromTos[i].Amount = &remaining
		}

		scdt = append(scdt, AliasKey(fromTos[i].SplitAlias(), fromTos[i].BalanceKey))
	}

	t <- total

	ft <- fmto

	sd <- scdt

	or <- operationRoute
}

// AppendIfNotExist Append if not exist
func AppendIfNotExist(slice []string, s []string) []string {
	for _, v := range s {
		if !commons.Contains(slice, v) {
			slice = append(slice, v)
		}
	}

	return slice
}

// ValidateSendSourceAndDistribute Validate send and distribute totals
func ValidateSendSourceAndDistribute(ctx context.Context, transaction Transaction, transactionType string) (*Responses, error) {
	var (
		sourcesTotal      decimal.Decimal
		destinationsTotal decimal.Decimal
	)

	logger, tracer, _, _ := commons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "commons.transaction.ValidateSendSourceAndDistribute")
	defer span.End()

	sizeFrom := len(transaction.Send.Source.From)
	sizeTo := len(transaction.Send.Distribute.To)

	response := &Responses{
		Total:               transaction.Send.Value,
		Asset:               transaction.Send.Asset,
		From:                make(map[string]Amount, sizeFrom),
		To:                  make(map[string]Amount, sizeTo),
		Sources:             make([]string, 0, sizeFrom),
		Destinations:        make([]string, 0, sizeTo),
		Aliases:             make([]string, 0, sizeFrom+sizeTo),
		Pending:             transaction.Pending,
		TransactionRoute:    transaction.Route,
		OperationRoutesFrom: make(map[string]string, sizeFrom),
		OperationRoutesTo:   make(map[string]string, sizeTo),
	}

	tFrom := make(chan decimal.Decimal, sizeFrom)
	ftFrom := make(chan map[string]Amount, sizeFrom)
	sdFrom := make(chan []string, sizeFrom)
	orFrom := make(chan map[string]string, sizeFrom)

	go CalculateTotal(transaction.Send.Source.From, transaction, transactionType, tFrom, ftFrom, sdFrom, orFrom)

	sourcesTotal = <-tFrom
	response.From = <-ftFrom
	response.Sources = <-sdFrom
	response.OperationRoutesFrom = <-orFrom
	response.Aliases = AppendIfNotExist(response.Aliases, response.Sources)

	tTo := make(chan decimal.Decimal, sizeTo)
	ftTo := make(chan map[string]Amount, sizeTo)
	sdTo := make(chan []string, sizeTo)
	orTo := make(chan map[string]string, sizeTo)

	go CalculateTotal(transaction.Send.Distribute.To, transaction, transactionType, tTo, ftTo, sdTo, orTo)

	destinationsTotal = <-tTo
	response.To = <-ftTo
	response.Destinations = <-sdTo
	response.OperationRoutesTo = <-orTo
	response.Aliases = AppendIfNotExist(response.Aliases, response.Destinations)

	for i, source := range response.Sources {
		if _, ok := response.To[ConcatAlias(i, source)]; ok {
			logger.Log(ctx, libLog.LevelError, "ValidateSendSourceAndDistribute: Ambiguous transaction source and destination")

			return nil, commons.ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute")
		}
	}

	for i, destination := range response.Destinations {
		if _, ok := response.From[ConcatAlias(i, destination)]; ok {
			logger.Log(ctx, libLog.LevelError, "ValidateSendSourceAndDistribute: Ambiguous transaction source and destination")

			return nil, commons.ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute")
		}
	}

	if !sourcesTotal.Equal(destinationsTotal) || !destinationsTotal.Equal(response.Total) {
		logger.Log(ctx, libLog.LevelError, "ValidateSendSourceAndDistribute: Transaction value mismatch")

		return nil, commons.ValidateBusinessError(constant.ErrTransactionValueMismatch, "ValidateSendSourceAndDistribute")
	}

	return response, nil
}
