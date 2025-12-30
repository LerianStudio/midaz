package transaction

import (
	"context"
	"strconv"
	"strings"

	"github.com/LerianStudio/lib-commons/v2/commons"
	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	localConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
)

const (
	// percentageMultiplier is used to convert percentages to decimal values (divide by 100)
	percentageMultiplier = 100
)

// ValidateBalancesRules function with some validates in accounts and DSL operations
func ValidateBalancesRules(ctx context.Context, transaction Transaction, validate Responses, balances []*Balance) error {
	logger, tracer, _, _ := commons.NewTrackingFromContext(ctx)

	_, spanValidateBalances := tracer.Start(ctx, "validations.validate_balances_rules")
	defer spanValidateBalances.End()

	if len(balances) != (len(validate.From) + len(validate.To)) {
		err := commons.ValidateBusinessError(constant.ErrAccountIneligibility, "ValidateAccounts")

		opentelemetry.HandleSpanBusinessErrorEvent(&spanValidateBalances, "validations.validate_balances_rules", err)

		//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
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
				//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
				return commons.ValidateBusinessError(constant.ErrAssetCodeNotFound, "validateFromAccounts")
			}

			if !balance.AllowSending {
				//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
				return commons.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateFromAccounts")
			}

			if pending && balance.AccountType == constant.ExternalAccountType {
				//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
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
				//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
				return commons.ValidateBusinessError(constant.ErrAssetCodeNotFound, "validateToAccounts")
			}

			if !balance.AllowReceiving {
				//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
				return commons.ValidateBusinessError(constant.ErrAccountStatusTransactionRestriction, "validateToAccounts")
			}

			if balance.Available.IsPositive() && balance.AccountType == constant.ExternalAccountType {
				//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
				return commons.ValidateBusinessError(constant.ErrInsufficientFunds, "validateToAccounts", balance.Alias)
			}
		}
	}

	return nil
}

// findAmountByAlias looks up an Amount in a map by alias, handling key format mismatch.
// Keys in the map may be in concatenated format "index#alias#balanceKey" (e.g., "0#@external/USD#default")
// while the lookup alias may be in simple format (e.g., "@external/USD").
func findAmountByAlias(m map[string]Amount, alias string) Amount {
	// Try direct lookup first
	if amt, ok := m[alias]; ok {
		return amt
	}

	// Try to find concatenated key containing this alias
	// Concatenated format: "index#alias#balanceKey"
	for key, amt := range m {
		parts := strings.Split(key, "#")
		if len(parts) >= 2 && parts[1] == alias {
			return amt
		}
	}

	return Amount{}
}

// ValidateFromToOperation func that validate operate balance
func ValidateFromToOperation(ft FromTo, validate Responses, balance *Balance) (Amount, Balance, error) {
	if !ft.IsFrom {
		amt := findAmountByAlias(validate.To, ft.AccountAlias)

		ba, err := OperateBalances(amt, *balance)
		if err != nil {
			return Amount{}, Balance{}, err
		}

		return amt, ba, nil
	}

	amt := findAmountByAlias(validate.From, ft.AccountAlias)

	ba, err := OperateBalances(amt, *balance)
	if err != nil {
		return Amount{}, Balance{}, err
	}

	isInsufficientFunds := ba.Available.IsNegative() && balance.AccountType != constant.ExternalAccountType
	if isInsufficientFunds {
		//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
		return Amount{}, Balance{}, commons.ValidateBusinessError(constant.ErrInsufficientFunds, "ValidateFromToOperation", balance.Alias)
	}

	return amt, ba, nil
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

// applyPendingOperation applies pending transaction operations
func applyPendingOperation(amount Amount, available, onHold decimal.Decimal) (decimal.Decimal, decimal.Decimal, bool) {
	if amount.Operation == constant.ONHOLD {
		return available.Sub(amount.Value), onHold.Add(amount.Value), true
	}

	return available, onHold, false
}

// applyCanceledOperation applies canceled transaction operations
func applyCanceledOperation(amount Amount, available, onHold decimal.Decimal) (decimal.Decimal, decimal.Decimal, bool) {
	if amount.Operation == constant.RELEASE {
		newOnHold := onHold.Sub(amount.Value)
		// OnHold can never be negative - this indicates a programming error
		// (trying to release more than was held)
		assert.That(assert.NonNegativeDecimal(newOnHold),
			"onHold cannot go negative during RELEASE",
			"original_onHold", onHold.String(),
			"release_amount", amount.Value.String(),
			"result", newOnHold.String())
		return available.Add(amount.Value), newOnHold, true
	}

	return available, onHold, false
}

// applyApprovedOperation applies approved transaction operations
func applyApprovedOperation(amount Amount, available, onHold decimal.Decimal) (decimal.Decimal, decimal.Decimal, bool) {
	if amount.Operation == constant.DEBIT {
		return available, onHold.Sub(amount.Value), true
	}

	if amount.Operation == constant.CREDIT {
		return available.Add(amount.Value), onHold, true
	}

	return available, onHold, false
}

// applyCreatedOperation applies created transaction operations
func applyCreatedOperation(amount Amount, available, onHold decimal.Decimal) (decimal.Decimal, decimal.Decimal, bool) {
	if amount.Operation == constant.DEBIT {
		return available.Sub(amount.Value), onHold, true
	}

	if amount.Operation == constant.CREDIT {
		return available.Add(amount.Value), onHold, true
	}

	return available, onHold, false
}

// applyBalanceOperation applies a specific operation to balance amounts
func applyBalanceOperation(amount Amount, available, onHold decimal.Decimal) (decimal.Decimal, decimal.Decimal, bool) {
	switch amount.TransactionType {
	case constant.PENDING:
		return applyPendingOperation(amount, available, onHold)
	case constant.CANCELED:
		return applyCanceledOperation(amount, available, onHold)
	case constant.APPROVED:
		return applyApprovedOperation(amount, available, onHold)
	case constant.CREATED:
		return applyCreatedOperation(amount, available, onHold)
	case localConstant.NOTED:
		// Annotation/no-op transactions must not affect balances.
		return available, onHold, false
	default:
		// Note: operation is the balance operation type (DEBIT/CREDIT), distinct from transactionType
		assert.Never("unhandled transaction type in applyBalanceOperation",
			"transactionType", amount.TransactionType,
			"operation", amount.Operation,
			"value", amount.Value.String())

		return available, onHold, false // unreachable, satisfies compiler
	}
}

// OperateBalances Function to sum or sub two balances and Normalize the scale
func OperateBalances(amount Amount, balance Balance) (Balance, error) {
	// Validate input precision - catches malformed amounts before processing
	assert.That(assert.ValidAmount(amount.Value),
		"amount value has invalid precision",
		"value", amount.Value.String(),
		"exponent", amount.Value.Exponent())

	total, totalOnHold, changed := applyBalanceOperation(amount, balance.Available, balance.OnHold)

	if !changed {
		// For no-op transactions (e.g., NOTED), return the original balance without changing the version.
		return balance, nil
	}

	// Validate output precision - ensures results are within valid bounds
	assert.That(assert.ValidAmount(total),
		"resulting available has invalid precision",
		"value", total.String(),
		"exponent", total.Exponent())
	assert.That(assert.ValidAmount(totalOnHold),
		"resulting onHold has invalid precision",
		"value", totalOnHold.String(),
		"exponent", totalOnHold.Exponent())

	newVersion := balance.Version + 1
	assert.That(assert.Positive(newVersion),
		"balance version must be positive after increment",
		"previousVersion", balance.Version,
		"newVersion", newVersion)

	return Balance{
		Available: total,
		OnHold:    totalOnHold,
		Version:   newVersion,
	}, nil
}

// determineOperationForPendingTransaction determines the operation for pending transactions
func determineOperationForPendingTransaction(isFrom bool, transactionType string) string {
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
		if isFrom {
			return constant.DEBIT
		}

		return constant.CREDIT
	default:
		assert.Never("unhandled transaction type in determineOperationForPendingTransaction",
			"transactionType", transactionType,
			"isFrom", isFrom)

		return constant.CREDIT // unreachable, satisfies compiler
	}
}

// DetermineOperation Function to determine the operation
func DetermineOperation(isPending bool, isFrom bool, transactionType string) string {
	if isPending {
		return determineOperationForPendingTransaction(isFrom, transactionType)
	}

	if isFrom {
		return constant.DEBIT
	}

	return constant.CREDIT
}

// CalculateTotal Calculate total for sources/destinations based on shares, amounts and remains
func CalculateTotal(fromTos []FromTo, transaction Transaction, transactionType string) (
	total decimal.Decimal,
	amounts map[string]Amount,
	aliases []string,
	operationRoutes map[string]string,
) {
	amounts = make(map[string]Amount)
	aliases = make([]string, 0)
	operationRoutes = make(map[string]string)

	assert.That(assert.ValidTransactionStatus(transactionType),
		"transaction type must be valid",
		"transactionType", transactionType)

	total = decimal.NewFromInt(0)

	remaining := Amount{
		Asset:           transaction.Send.Asset,
		Value:           transaction.Send.Value,
		TransactionType: transactionType,
	}

	// Track total share percentage and remaining count for validation
	var totalSharePercentage int64 = 0
	anyShareUsed := false
	remainingCount := 0

	for i := range fromTos {
		operationRoutes[fromTos[i].AccountAlias] = fromTos[i].Route

		operation := DetermineOperation(transaction.Pending, fromTos[i].IsFrom, transactionType)

		assert.That(!(fromTos[i].Amount != nil && fromTos[i].Share != nil),
			"from/to entry cannot contain both amount and share",
			"alias", fromTos[i].AccountAlias)

		amountAsset := ""
		if fromTos[i].Amount != nil {
			amountAsset = fromTos[i].Amount.Asset
		}
		assert.That(fromTos[i].Amount == nil || amountAsset == transaction.Send.Asset,
			"amount asset must match transaction asset",
			"alias", fromTos[i].AccountAlias,
			"amount_asset", amountAsset,
			"transaction_asset", transaction.Send.Asset)

		if fromTos[i].Rate != nil {
			assert.That(fromTos[i].Rate.From != "" && fromTos[i].Rate.To != "",
				"rate from/to must be set",
				"alias", fromTos[i].AccountAlias)
			assert.That(fromTos[i].Rate.Value.IsPositive(),
				"rate value must be positive",
				"alias", fromTos[i].AccountAlias)
		}

		if fromTos[i].Share != nil {
			assert.That(assert.InRange(fromTos[i].Share.Percentage, 0, 100),
				"share percentage must be 0-100",
				"alias", fromTos[i].AccountAlias)
			assert.That(fromTos[i].Share.PercentageOfPercentage == 0 || assert.InRange(fromTos[i].Share.PercentageOfPercentage, 0, 100),
				"percentageOfPercentage must be 0-100",
				"alias", fromTos[i].AccountAlias)
		}

		if fromTos[i].Share != nil && fromTos[i].Share.Percentage != 0 {
			anyShareUsed = true
			// Accumulate share percentages
			totalSharePercentage += fromTos[i].Share.Percentage
			assert.That(totalSharePercentage <= 100,
				"total share percentages cannot exceed 100",
				"total_percentage", totalSharePercentage)

			oneHundred := decimal.NewFromInt(percentageMultiplier)

			percentage := decimal.NewFromInt(fromTos[i].Share.Percentage)

			percentageOfPercentage := decimal.NewFromInt(fromTos[i].Share.PercentageOfPercentage)
			if percentageOfPercentage.IsZero() {
				percentageOfPercentage = oneHundred
			}

			firstPart := percentage.Div(oneHundred)
			secondPart := percentageOfPercentage.Div(oneHundred)
			shareValue := transaction.Send.Value.Mul(firstPart).Mul(secondPart)

			amounts[fromTos[i].AccountAlias] = Amount{
				Asset:           transaction.Send.Asset,
				Value:           shareValue,
				Operation:       operation,
				TransactionType: transactionType,
			}

			total = total.Add(shareValue)
			remaining.Value = remaining.Value.Sub(shareValue)

			// Assert remaining never goes negative during distribution
			assert.That(assert.NonNegativeDecimal(remaining.Value),
				"remaining value cannot go negative during distribution",
				"index", i,
				"remaining", remaining.Value.String(),
				"accountAlias", fromTos[i].AccountAlias)
		}

		if fromTos[i].Amount != nil && fromTos[i].Amount.Value.IsPositive() {
			amount := Amount{
				Asset:           fromTos[i].Amount.Asset,
				Value:           fromTos[i].Amount.Value,
				Operation:       operation,
				TransactionType: transactionType,
			}

			amounts[fromTos[i].AccountAlias] = amount
			total = total.Add(amount.Value)

			remaining.Value = remaining.Value.Sub(amount.Value)
		}

		if !commons.IsNilOrEmpty(&fromTos[i].Remaining) {
			remainingCount++
			assert.That(remainingCount <= 1,
				"only one remaining entry allowed",
				"count", remainingCount)

			total = total.Add(remaining.Value)

			remaining.Operation = operation

			amounts[fromTos[i].AccountAlias] = remaining
			fromTos[i].Amount = &remaining
		}

		aliases = append(aliases, AliasKey(fromTos[i].SplitAlias(), fromTos[i].BalanceKey))
	}

	// Assert total shares don't exceed 100%
	assert.That(totalSharePercentage <= 100,
		"total share percentages cannot exceed 100",
		"total_percentage", totalSharePercentage)
	assert.That(!anyShareUsed || totalSharePercentage == 100 || remainingCount == 1,
		"remaining entry required when share total < 100",
		"total_share", totalSharePercentage)

	return total, amounts, aliases, operationRoutes
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

	// Calculate totals synchronously - no goroutines needed for sequential blocking calls
	var sourcesTotal, destinationsTotal decimal.Decimal

	sourcesTotal, response.From, response.Sources, response.OperationRoutesFrom = CalculateTotal(transaction.Send.Source.From, transaction, transactionType)
	response.Aliases = AppendIfNotExist(response.Aliases, response.Sources)

	destinationsTotal, response.To, response.Destinations, response.OperationRoutesTo = CalculateTotal(transaction.Send.Distribute.To, transaction, transactionType)
	response.Aliases = AppendIfNotExist(response.Aliases, response.Destinations)

	for i, source := range response.Sources {
		if _, ok := response.To[ConcatAlias(i, source)]; ok {
			logger.Errorf("ValidateSendSourceAndDistribute: Ambiguous transaction source and destination")

			//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
			return nil, commons.ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute")
		}
	}

	for i, destination := range response.Destinations {
		if _, ok := response.From[ConcatAlias(i, destination)]; ok {
			logger.Errorf("ValidateSendSourceAndDistribute: Ambiguous transaction source and destination")

			//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
			return nil, commons.ValidateBusinessError(constant.ErrTransactionAmbiguous, "ValidateSendSourceAndDistribute")
		}
	}

	if !sourcesTotal.Equal(destinationsTotal) || !destinationsTotal.Equal(response.Total) {
		logger.Errorf("ValidateSendSourceAndDistribute: Transaction value mismatch")

		//nolint:wrapcheck // ValidateBusinessError already returns a properly formatted business error with context
		return nil, commons.ValidateBusinessError(constant.ErrTransactionValueMismatch, "ValidateSendSourceAndDistribute")
	}

	return response, nil
}
