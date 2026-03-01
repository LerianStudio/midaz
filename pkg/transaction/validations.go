// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/LerianStudio/lib-commons/v2/commons"
	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
)

// oneHundred is the percentage base used in share calculations (100%).
const oneHundred = 100

// ValidateBalancesRules function with some validates in accounts operations.
func ValidateBalancesRules(ctx context.Context, transaction Transaction, validate Responses, balances []*Balance) error {
	logger, tracer, _, _ := commons.NewTrackingFromContext(ctx)

	_, spanValidateBalances := tracer.Start(ctx, "validations.validate_balances_rules")
	defer spanValidateBalances.End()

	if len(balances) != (len(validate.From) + len(validate.To)) {
		err := fmt.Errorf("balance count mismatch: %w",
			commons.ValidateBusinessError(constant.ErrAccountIneligibility, "ValidateAccounts"))

		opentelemetry.HandleSpanBusinessErrorEvent(&spanValidateBalances, "validations.validate_balances_rules", err)

		return err
	}

	for _, balance := range balances {
		if err := validateFromBalances(balance, validate.From, validate.Asset, validate.Pending); err != nil {
			opentelemetry.HandleSpanBusinessErrorEvent(&spanValidateBalances, "validations.validate_from_balances_", err)

			logger.Errorf("validations.validate_from_balances_err: %s", err)

			return err
		}

		if err := validateToBalances(balance, validate.To, validate.Asset); err != nil {
			opentelemetry.HandleSpanBusinessErrorEvent(&spanValidateBalances, "validations.validate_to_balances_", err)

			logger.Errorf("validations.validate_to_balances_err: %s", err)

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
				return fmt.Errorf("from balance asset mismatch: %w",
					commons.ValidateBusinessError(constant.ErrAssetCodeNotFound, "validateFromAccounts"))
			}

			if !balance.AllowSending {
				return fmt.Errorf("from balance sending restricted: %w",
					commons.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateFromAccounts"))
			}

			if pending && balance.AccountType == constant.ExternalAccountType {
				return fmt.Errorf("on-hold external account: %w",
					commons.ValidateBusinessError(constant.ErrOnHoldExternalAccount, "validateBalance", balance.Alias))
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
				return fmt.Errorf("to balance asset mismatch: %w",
					commons.ValidateBusinessError(constant.ErrAssetCodeNotFound, "validateToAccounts"))
			}

			if !balance.AllowReceiving {
				return fmt.Errorf("to balance receiving restricted: %w",
					commons.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateToAccounts"))
			}
		}
	}

	return nil
}

// ValidateFromToOperation func that validate operate balance.
func ValidateFromToOperation(ft FromTo, validate Responses, balance *Balance) (Amount, Balance, error) {
	if ft.IsFrom {
		ba, err := OperateBalances(validate.From[ft.AccountAlias], *balance)
		if err != nil {
			return Amount{}, Balance{}, err
		}

		return validate.From[ft.AccountAlias], ba, nil
	}

	ba, err := OperateBalances(validate.To[ft.AccountAlias], *balance)
	if err != nil {
		return Amount{}, Balance{}, err
	}

	return validate.To[ft.AccountAlias], ba, nil
}

// AliasKey function to concatenate alias with balance key.
func AliasKey(alias, balanceKey string) string {
	if balanceKey == "" {
		balanceKey = "default"
	}

	return alias + "#" + balanceKey
}

// SplitAlias function to split alias with index.
func SplitAlias(alias string) string {
	if strings.Contains(alias, "#") {
		return strings.Split(alias, "#")[1]
	}

	return alias
}

// ConcatAlias function to concat alias with index.
func ConcatAlias(i int, alias string) string {
	return strconv.Itoa(i) + "#" + alias
}

// OperateBalances Function to sum or sub two balances and Normalize the scale.
func OperateBalances(amount Amount, balance Balance) (Balance, error) {
	var (
		total        decimal.Decimal
		totalOnHold  decimal.Decimal
		totalVersion int64
	)

	total = balance.Available
	totalOnHold = balance.OnHold

	switch {
	case amount.Operation == constant.ONHOLD && amount.TransactionType == constant.PENDING:
		total = balance.Available.Sub(amount.Value)
		totalOnHold = balance.OnHold.Add(amount.Value)
	case amount.Operation == constant.RELEASE && (amount.TransactionType == constant.CANCELED || amount.TransactionType == constant.APPROVED):
		totalOnHold = balance.OnHold.Sub(amount.Value)
		total = balance.Available.Add(amount.Value)
	case amount.Operation == constant.DEBIT && amount.TransactionType == constant.APPROVED:
		totalOnHold = balance.OnHold.Sub(amount.Value)
	case amount.Operation == constant.CREDIT && amount.TransactionType == constant.APPROVED:
		total = balance.Available.Add(amount.Value)
	case amount.Operation == constant.DEBIT && amount.TransactionType == constant.CREATED:
		total = balance.Available.Sub(amount.Value)
	case amount.Operation == constant.CREDIT && amount.TransactionType == constant.CREATED:
		total = balance.Available.Add(amount.Value)
	default:
		// For unknown operations, return the original balance without changing the version.
		return balance, nil
	}

	totalVersion = balance.Version + 1

	return Balance{
		Available: total,
		OnHold:    totalOnHold,
		Version:   totalVersion,
	}, nil
}

// DetermineOperation returns the balance operation type (DEBIT, CREDIT, ONHOLD, RELEASE)
// based on whether the transaction is pending, the entry direction, and the transaction type.
func DetermineOperation(isPending, isFrom bool, transactionType string) string {
	if !isPending {
		return debitOrCredit(isFrom)
	}

	return determinePendingOperation(isFrom, transactionType)
}

// determinePendingOperation resolves the operation for pending transactions.
func determinePendingOperation(isFrom bool, transactionType string) string {
	switch transactionType {
	case constant.PENDING:
		if isFrom {
			return constant.ONHOLD
		}

		return constant.CREDIT
	case constant.CANCELED:
		if isFrom {
			return constant.RELEASE
		}

		return constant.CREDIT
	case constant.APPROVED:
		return debitOrCredit(isFrom)
	default:
		return constant.CREDIT
	}
}

// debitOrCredit returns DEBIT for source entries and CREDIT for destination entries.
func debitOrCredit(isFrom bool) string {
	if isFrom {
		return constant.DEBIT
	}

	return constant.CREDIT
}

// CalculateTotal Calculate total for sources/destinations based on shares, amounts and remains.
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
		operationRoute[fromTos[i].AccountAlias] = fromTos[i].Route

		operation := DetermineOperation(transaction.Pending, fromTos[i].IsFrom, transactionType)

		if fromTos[i].Share != nil && fromTos[i].Share.Percentage != 0 {
			oneHundred := decimal.NewFromInt(oneHundred)

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
			}

			fmto[fromTos[i].AccountAlias] = amount
			total = total.Add(amount.Value)

			remaining.Value = remaining.Value.Sub(amount.Value)
		}

		if !commons.IsNilOrEmpty(&fromTos[i].Remaining) {
			total = total.Add(remaining.Value)

			remaining.Operation = operation

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

// AppendIfNotExist Append if not exist.
func AppendIfNotExist(slice, s []string) []string {
	for _, v := range s {
		if !commons.Contains(slice, v) {
			slice = append(slice, v)
		}
	}

	return slice
}

// ValidateSendSourceAndDistribute Validate send and distribute totals.
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
			logger.Errorf("ValidateSendSourceAndDistribute: Ambiguous transaction source and destination")

			return nil, fmt.Errorf("ambiguous source/destination: %w",
				commons.ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute"))
		}
	}

	for i, destination := range response.Destinations {
		if _, ok := response.From[ConcatAlias(i, destination)]; ok {
			logger.Errorf("ValidateSendSourceAndDistribute: Ambiguous transaction source and destination")

			return nil, fmt.Errorf("ambiguous source/destination: %w",
				commons.ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute"))
		}
	}

	if !sourcesTotal.Equal(destinationsTotal) || !destinationsTotal.Equal(response.Total) {
		logger.Errorf("ValidateSendSourceAndDistribute: Transaction value mismatch")

		return nil, fmt.Errorf("transaction value mismatch: %w",
			commons.ValidateBusinessError(constant.ErrTransactionValueMismatch, "ValidateSendSourceAndDistribute"))
	}

	return response, nil
}
