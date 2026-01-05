package transaction

import (
	"context"
	"strconv"
	"strings"

	"github.com/LerianStudio/lib-commons/v2/commons"
	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libmetrics "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry/metrics"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	localConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"
)

const (
	// percentageMultiplier is used to convert percentages to decimal values (divide by 100)
	percentageMultiplier = 100
	// maxPercentage is the maximum valid percentage value (100%).
	maxPercentage = 100
	// maxDecimalPlaces is the maximum number of decimal places for share calculations.
	maxDecimalPlaces = 18
)

var (
	// balanceOperationIdempotencySkipMetric tracks when we short-circuit balance changes due to idempotency checks.
	// This helps distinguish legitimate retries (e.g., RabbitMQ redelivery) from unexpected call order bugs
	// (e.g., RELEASE without a prior ONHOLD).
	balanceOperationIdempotencySkipMetric = libmetrics.Metric{
		Name:        "balance_operation_idempotency_skip_total",
		Unit:        "1",
		Description: "Total number of times a balance operation was skipped due to idempotency short-circuit checks",
	}

	// transactionsIdempotentDebitMetric tracks approved DEBIT idempotency skips specifically.
	// This is useful to distinguish retry-driven reprocessing from unexpected flows.
	transactionsIdempotentDebitMetric = libmetrics.Metric{
		Name:        "transactions_idempotent_debit_total",
		Unit:        "1",
		Description: "Total number of times an approved DEBIT was skipped because onHold was zero (already applied)",
	}

	// balanceOperationClampMetric tracks when we clamp a positive onHold that is less than a DEBIT amount to zero.
	// This indicates an unexpected partial-hold state (often retry-related) that should be traceable.
	balanceOperationClampMetric = libmetrics.Metric{
		Name:        "balance_operation_clamp_total",
		Unit:        "1",
		Description: "Total number of times balance onHold was clamped to zero because it was positive but less than the DEBIT amount",
	}
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
		// Idempotency check: if onHold is zero, the release was already applied
		// (e.g., during a successful previous attempt before RabbitMQ retry).
		// Return unchanged to avoid double-processing.
		if onHold.IsZero() {
			return available, onHold, false
		}

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
		// Idempotency check: if onHold is zero, the debit was already applied
		// (e.g., during a successful previous attempt before RabbitMQ retry).
		// Return unchanged to avoid double-processing.
		if onHold.IsZero() {
			return available, onHold, false
		}

		// Convergence/safety: if onHold is positive but less than the debit amount,
		// clamp to zero instead of going negative. This can happen during retries
		// where part of the hold was already consumed by a previous successful attempt.
		if onHold.LessThan(amount.Value) {
			return available, decimal.Zero, true
		}

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

func isBalanceIdempotencySkip(amount Amount, balance Balance) (bool, string) {
	// RELEASE idempotency: balance already has no on-hold funds, so a retry must not double-apply.
	if amount.TransactionType == constant.CANCELED &&
		amount.Operation == constant.RELEASE &&
		balance.OnHold.IsZero() {
		return true, "release_onhold_zero"
	}

	// DEBIT idempotency: on-hold already consumed, so a retry must not double-apply.
	if amount.TransactionType == constant.APPROVED &&
		amount.Operation == constant.DEBIT &&
		balance.OnHold.IsZero() {
		return true, "debit_onhold_zero"
	}

	return false, ""
}

func isBalanceClamp(amount Amount, balance Balance) (bool, string) {
	// Clamp case: convergence/safety during retries where onHold was partially consumed by a prior successful attempt.
	if amount.TransactionType == constant.APPROVED &&
		amount.Operation == constant.DEBIT &&
		balance.OnHold.IsPositive() &&
		balance.OnHold.LessThan(amount.Value) {
		return true, "debit_onhold_lt_amount"
	}

	return false, ""
}

// OperateBalancesWithContext is a context-aware wrapper around OperateBalances.
// It adds observability for idempotency short-circuit paths without changing the
// core balance math or versioning behavior.
func OperateBalancesWithContext(ctx context.Context, transactionID string, amount Amount, balance Balance) (Balance, error) {
	logger, _, reqID, metricFactory := commons.NewTrackingFromContext(ctx)

	if ok, reason := isBalanceIdempotencySkip(amount, balance); ok {
		recordIdempotencySkipMetrics(ctx, metricFactory, amount, reason)
		logIdempotencySkip(logger, transactionID, amount, balance, reason)
	}

	if ok, reason := isBalanceClamp(amount, balance); ok {
		recordClampMetrics(ctx, metricFactory, amount, reason)
		logBalanceClamp(ctx, logger, transactionID, amount, balance, reason, reqID)
	}

	return OperateBalances(amount, balance)
}

// recordIdempotencySkipMetrics records metrics for idempotency skip events.
func recordIdempotencySkipMetrics(ctx context.Context, metricFactory *libmetrics.MetricsFactory, amount Amount, reason string) {
	if metricFactory == nil {
		return
	}

	metricFactory.Counter(balanceOperationIdempotencySkipMetric).
		WithLabels(map[string]string{
			"transaction_type": amount.TransactionType,
			"operation":        amount.Operation,
			"reason":           reason,
		}).
		AddOne(ctx)

	if reason == "debit_onhold_zero" {
		metricFactory.Counter(transactionsIdempotentDebitMetric).
			AddOne(ctx)
	}
}

// logIdempotencySkip logs debug information for idempotency skip events.
func logIdempotencySkip(logger log.Logger, transactionID string, amount Amount, balance Balance, reason string) {
	if logger == nil {
		return
	}

	logger.Debugf(
		"balance_operation_idempotency_skip: transaction_id=%s account_id=%s txType=%s op=%s reason=%s balanceAlias=%s balanceKey=%s",
		transactionID, balance.AccountID, amount.TransactionType, amount.Operation, reason, balance.Alias, balance.Key,
	)
}

// recordClampMetrics records metrics for balance clamp events.
func recordClampMetrics(ctx context.Context, metricFactory *libmetrics.MetricsFactory, amount Amount, reason string) {
	if metricFactory == nil {
		return
	}

	metricFactory.Counter(balanceOperationClampMetric).
		WithLabels(map[string]string{
			"transaction_type": amount.TransactionType,
			"operation":        amount.Operation,
			"reason":           reason,
		}).
		AddOne(ctx)
}

// logBalanceClamp logs warning information for balance clamp events.
func logBalanceClamp(ctx context.Context, logger log.Logger, transactionID string, amount Amount, balance Balance, reason, reqID string) {
	if logger == nil {
		return
	}

	traceID, spanID := extractTraceInfo(ctx)

	logger.Warnf(
		"balance_operation_clamp: transaction_id=%s account_id=%s txType=%s op=%s reason=%s amount=%s onHold=%s request_id=%s trace_id=%s span_id=%s balanceAlias=%s balanceKey=%s",
		transactionID,
		balance.AccountID,
		amount.TransactionType,
		amount.Operation,
		reason,
		amount.Value.String(),
		balance.OnHold.String(),
		reqID,
		traceID,
		spanID,
		balance.Alias,
		balance.Key,
	)
}

// extractTraceInfo extracts trace and span IDs from context.
func extractTraceInfo(ctx context.Context) (traceID, spanID string) {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		return spanCtx.TraceID().String(), spanCtx.SpanID().String()
	}

	return "", ""
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

// validateFromToEntry validates constraints on amount, share, and rate for a FromTo entry.
func validateFromToEntry(ft *FromTo, sendAsset string) {
	assert.That(ft.Amount == nil || ft.Share == nil,
		"from/to entry cannot contain both amount and share",
		"alias", ft.AccountAlias)

	amountAsset := ""
	if ft.Amount != nil {
		amountAsset = ft.Amount.Asset
	}

	assert.That(ft.Amount == nil || amountAsset == sendAsset,
		"amount asset must match transaction asset",
		"alias", ft.AccountAlias,
		"amount_asset", amountAsset,
		"transaction_asset", sendAsset)

	if ft.Rate != nil {
		assert.That(ft.Rate.From != "" && ft.Rate.To != "",
			"rate from/to must be set",
			"alias", ft.AccountAlias)
		assert.That(ft.Rate.Value.IsPositive(),
			"rate value must be positive",
			"alias", ft.AccountAlias)
	}

	if ft.Share != nil {
		assert.That(assert.InRange(ft.Share.Percentage, 0, maxPercentage),
			"share percentage must be 0-100",
			"alias", ft.AccountAlias)
		assert.That(ft.Share.PercentageOfPercentage == 0 || assert.InRange(ft.Share.PercentageOfPercentage, 0, maxPercentage),
			"percentageOfPercentage must be 0-100",
			"alias", ft.AccountAlias)
	}
}

// calculateShareValue computes the value for a share-based FromTo entry.
func calculateShareValue(share *Share, sendValue decimal.Decimal) decimal.Decimal {
	oneHundred := decimal.NewFromInt(percentageMultiplier)
	percentage := decimal.NewFromInt(share.Percentage)
	percentageOfPercentage := decimal.NewFromInt(share.PercentageOfPercentage)

	if percentageOfPercentage.IsZero() {
		percentageOfPercentage = oneHundred
	}

	firstPart := percentage.Div(oneHundred)
	secondPart := percentageOfPercentage.Div(oneHundred)
	// Truncate to 18 decimal places to ensure exponent stays within valid range [-18, 18].
	return sendValue.Mul(firstPart).Mul(secondPart).Truncate(maxDecimalPlaces)
}

// processShareEntry processes a FromTo entry with share-based distribution.
func processShareEntry(
	ft *FromTo, operation, transactionType, sendAsset string, sendValue decimal.Decimal,
	amounts map[string]Amount, total *decimal.Decimal, remaining *Amount,
	totalSharePercentage *int64, idx int,
) {
	*totalSharePercentage += ft.Share.Percentage
	assert.That(*totalSharePercentage <= maxPercentage,
		"total share percentages cannot exceed 100",
		"total_percentage", *totalSharePercentage)

	shareValue := calculateShareValue(ft.Share, sendValue)

	amounts[ft.AccountAlias] = Amount{
		Asset:           sendAsset,
		Value:           shareValue,
		Operation:       operation,
		TransactionType: transactionType,
	}

	*total = total.Add(shareValue)
	remaining.Value = remaining.Value.Sub(shareValue)

	assert.That(assert.NonNegativeDecimal(remaining.Value),
		"remaining value cannot go negative during distribution",
		"index", idx,
		"remaining", remaining.Value.String(),
		"accountAlias", ft.AccountAlias)
}

// processAmountEntry processes a FromTo entry with explicit amount.
func processAmountEntry(
	ft *FromTo, operation, transactionType string,
	amounts map[string]Amount, total *decimal.Decimal, remaining *Amount,
) {
	amount := Amount{
		Asset:           ft.Amount.Asset,
		Value:           ft.Amount.Value,
		Operation:       operation,
		TransactionType: transactionType,
	}

	amounts[ft.AccountAlias] = amount
	*total = total.Add(amount.Value)
	remaining.Value = remaining.Value.Sub(amount.Value)
}

// processRemainingEntry processes a FromTo entry that captures remaining value.
func processRemainingEntry(
	ft *FromTo, operation string, amounts map[string]Amount,
	total *decimal.Decimal, remaining *Amount, remainingCount *int,
) {
	*remainingCount++
	assert.That(*remainingCount <= 1,
		"only one remaining entry allowed",
		"count", *remainingCount)

	*total = total.Add(remaining.Value)
	remaining.Operation = operation
	amounts[ft.AccountAlias] = *remaining
	ft.Amount = remaining
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

	var totalSharePercentage int64

	anyShareUsed := false
	remainingCount := 0

	for i := range fromTos {
		operationRoutes[fromTos[i].AccountAlias] = fromTos[i].Route
		operation := DetermineOperation(transaction.Pending, fromTos[i].IsFrom, transactionType)
		validateFromToEntry(&fromTos[i], transaction.Send.Asset)

		if fromTos[i].Share != nil && fromTos[i].Share.Percentage != 0 {
			anyShareUsed = true

			processShareEntry(&fromTos[i], operation, transactionType, transaction.Send.Asset,
				transaction.Send.Value, amounts, &total, &remaining, &totalSharePercentage, i)
		}

		if fromTos[i].Amount != nil && fromTos[i].Amount.Value.IsPositive() {
			processAmountEntry(&fromTos[i], operation, transactionType, amounts, &total, &remaining)
		}

		if !commons.IsNilOrEmpty(&fromTos[i].Remaining) {
			processRemainingEntry(&fromTos[i], operation, amounts, &total, &remaining, &remainingCount)
		}

		aliases = append(aliases, AliasKey(fromTos[i].SplitAlias(), fromTos[i].BalanceKey))
	}

	assert.That(totalSharePercentage <= maxPercentage,
		"total share percentages cannot exceed 100",
		"total_percentage", totalSharePercentage)
	assert.That(!anyShareUsed || totalSharePercentage == maxPercentage || remainingCount == 1,
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
