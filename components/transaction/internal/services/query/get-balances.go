package query

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

const (
	maxBalanceLookupAttempts = 5
	balanceLookupBaseBackoff = 200 * time.Millisecond
)

// GetBalances methods responsible to get balances from a database.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL *pkgTransaction.Transaction, validate *pkgTransaction.Responses, transactionStatus string) ([]*mmodel.Balance, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(transactionID != uuid.Nil, "transactionID must not be nil UUID",
		"transactionID", transactionID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_balances")
	defer span.End()

	balances := make([]*mmodel.Balance, 0)

	balancesRedis, aliases := uc.ValidateIfBalanceExistsOnRedis(ctx, logger, organizationID, ledgerID, validate.Aliases)
	if len(balancesRedis) > 0 {
		balances = append(balances, balancesRedis...)
	}

	if len(aliases) > 0 {
		logger.Infof("DB_QUERY_START: Querying PostgreSQL for %d aliases: %v", len(aliases), aliases)

		queryStart := time.Now()

		balancesByAliases, err := uc.listBalancesByAliasesWithKeysWithRetry(ctx, organizationID, ledgerID, aliases, len(aliases), logger, time.Sleep)

		queryDuration := time.Since(queryStart)
		if err != nil {
			logger.Errorf("DB_QUERY_FAILED: PostgreSQL query failed after %v: %v", queryDuration, err)
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get account by alias on balance database", err)
			logger.Error("Failed to get account by alias on balance database", err.Error())

			// NOTE: This can return a non-nil, partial balance slice along with err.
			// Callers should treat a non-nil error as failure and decide whether partial results are acceptable.
			return append(balances, balancesByAliases...), err
		}

		logger.Infof("DB_QUERY_SUCCESS: PostgreSQL returned %d balances in %v", len(balancesByAliases), queryDuration)
		balances = append(balances, balancesByAliases...)
	}

	logger.Infof("REDIS_BALANCE_UPDATE_START: Starting Redis balance operations for %d balances", len(balances))

	lockStart := time.Now()

	newBalances, err := uc.GetAccountAndLock(ctx, organizationID, ledgerID, transactionID, parserDSL, validate, balances, transactionStatus)

	lockDuration := time.Since(lockStart)
	if err != nil {
		logger.Errorf("REDIS_BALANCE_UPDATE_FAILED: Failed to update balances after %v: %v", lockDuration, err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get balances and update on redis", err)

		logger.Error("Failed to get balances and update on redis", err.Error())

		return nil, err
	}

	logger.Infof("REDIS_BALANCE_UPDATE_SUCCESS: Successfully updated %d balances in %v", len(newBalances), lockDuration)

	return newBalances, nil
}

// listBalancesByAliasesWithKeysWithRetry retries balance lookup to handle transient connection issues during chaos scenarios.
//
// If expectedCount > 0, this enforces completeness: a successful query returning fewer than expectedCount balances is treated as incomplete and retried.
// If expectedCount == 0, completeness is NOT enforced and the first successful query result (even if partial) is returned without this strict check.
func (uc *UseCase) listBalancesByAliasesWithKeysWithRetry(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	aliases []string,
	expectedCount int,
	logger interface{ Warnf(string, ...any) },
	sleep func(time.Duration),
) ([]*mmodel.Balance, error) {
	// Retry balance lookup to handle transient connection issues during chaos scenarios
	// Attempts with exponential backoff: 200ms, 400ms, 800ms, 1600ms (total ~3s)
	var lastErr error

	for attempt := 0; attempt < maxBalanceLookupAttempts; attempt++ {
		balancesByAliases, err := uc.BalanceRepo.ListByAliasesWithKeys(ctx, organizationID, ledgerID, aliases)
		if err == nil {
			if expectedCount > 0 && len(balancesByAliases) < expectedCount {
				lastErr = fmt.Errorf("balances not ready: expected %d, got %d", expectedCount, len(balancesByAliases))
				if attempt == maxBalanceLookupAttempts-1 {
					return balancesByAliases, fmt.Errorf(
						"balances incomplete after max retries: expected %d, got %d: %w",
						expectedCount,
						len(balancesByAliases),
						lastErr,
					)
				}

				backoff := time.Duration(1<<attempt) * balanceLookupBaseBackoff
				logger.Warnf("Balance lookup incomplete (attempt %d/%d), retrying in %s: %v", attempt+1, maxBalanceLookupAttempts, backoff, lastErr)
				sleep(backoff)
				continue
			}

			return balancesByAliases, nil
		}

		lastErr = err
		if !isRetriableBalanceLookupErr(err) || attempt == maxBalanceLookupAttempts-1 {
			return nil, pkg.ValidateInternalError(lastErr, "Balance")
		}

		backoff := time.Duration(1<<attempt) * balanceLookupBaseBackoff
		logger.Warnf("Balance lookup failed (attempt %d/%d), retrying in %s: %v", attempt+1, maxBalanceLookupAttempts, backoff, err)
		sleep(backoff)
	}

	return nil, pkg.ValidateInternalError(lastErr, "Balance")
}

func isRetriableBalanceLookupErr(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	return strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline") ||
		strings.Contains(errStr, "unavailable")
}

// ValidateIfBalanceExistsOnRedis func that validate if balance exists on redis before to get on database.
func (uc *UseCase) ValidateIfBalanceExistsOnRedis(ctx context.Context, logger libLog.Logger, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []string) {
	_, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.validate_if_balance_exists_on_redis")
	defer span.End()

	newBalances := make([]*mmodel.Balance, 0)

	newAliases := make([]string, 0)

	for _, alias := range aliases {
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, alias)

		value, _ := uc.RedisRepo.Get(ctx, internalKey)
		if !libCommons.IsNilOrEmpty(&value) {
			b := mmodel.BalanceRedis{}

			if err := json.Unmarshal([]byte(value), &b); err != nil {
				libOpentelemetry.HandleSpanError(&span, "Error to Deserialization json", err)

				logger.Warnf("Error to Deserialization json: %v", err)

				continue
			}

			aliasAndKey := strings.Split(alias, "#")
			assert.That(len(aliasAndKey) == 2,
				"alias must contain exactly one '#' separator",
				"alias", alias,
				"parts", len(aliasAndKey))

			newBalances = append(newBalances, &mmodel.Balance{
				ID:             b.ID,
				AccountID:      b.AccountID,
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          aliasAndKey[0],
				Key:            aliasAndKey[1],
				Available:      b.Available,
				OnHold:         b.OnHold,
				Version:        b.Version,
				AccountType:    b.AccountType,
				AllowSending:   b.AllowSending == 1,
				AllowReceiving: b.AllowReceiving == 1,
				AssetCode:      b.AssetCode,
			})
		} else {
			newAliases = append(newAliases, alias)
		}
	}

	return newBalances, newAliases
}

// buildBalanceOperations creates balance operations from balances and validation data.
func buildBalanceOperations(organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance, validate *pkgTransaction.Responses) []mmodel.BalanceOperation {
	balanceOperations := make([]mmodel.BalanceOperation, 0)

	for _, balance := range balances {
		aliasKey := balance.Alias + "#" + balance.Key
		internalKey := utils.BalanceInternalKey(organizationID, ledgerID, aliasKey)

		balanceOperations = appendMatchingOperations(balanceOperations, balance, aliasKey, internalKey, validate.From)
		balanceOperations = appendMatchingOperations(balanceOperations, balance, aliasKey, internalKey, validate.To)
	}

	sort.Slice(balanceOperations, func(i, j int) bool {
		return balanceOperations[i].InternalKey < balanceOperations[j].InternalKey
	})

	return balanceOperations
}

// appendMatchingOperations appends balance operations for aliases that match the given aliasKey.
func appendMatchingOperations(operations []mmodel.BalanceOperation, balance *mmodel.Balance, aliasKey, internalKey string, aliasAmounts map[string]pkgTransaction.Amount) []mmodel.BalanceOperation {
	for k, v := range aliasAmounts {
		if pkgTransaction.SplitAliasWithKey(k) == aliasKey {
			operations = append(operations, mmodel.BalanceOperation{
				Balance:     balance,
				Alias:       k,
				Amount:      v,
				InternalKey: internalKey,
			})
		}
	}

	return operations
}

// GetAccountAndLock func responsible to integrate core business logic to redis.
func (uc *UseCase) GetAccountAndLock(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL *pkgTransaction.Transaction, validate *pkgTransaction.Responses, balances []*mmodel.Balance, transactionStatus string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "usecase.get_account_and_lock")
	defer span.End()

	balanceOperations := buildBalanceOperations(organizationID, ledgerID, balances, validate)

	err := uc.ValidateAccountingRules(ctx, organizationID, ledgerID, balanceOperations, validate)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate accounting rules", err)

		logger.Error("Failed to validate accounting rules", err)

		return nil, err
	}

	if parserDSL != nil {
		if err = uc.validateParserDSLBalances(ctx, &span, logger, parserDSL, validate, balanceOperations); err != nil {
			return nil, err
		}
	}

	balanceOperations = filterBalanceOperationsForStatus(balanceOperations, validate, transactionStatus)

	newBalances, err := uc.RedisRepo.AddSumBalancesRedis(ctx, organizationID, ledgerID, transactionID, transactionStatus, validate.Pending, balanceOperations)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to lock balance", err)

		logger.Error("Failed to lock balance", err)

		return nil, pkg.ValidateInternalError(err, "Balance")
	}

	return newBalances, nil
}

// validateParserDSLBalances validates balance rules when parserDSL is provided.
func (uc *UseCase) validateParserDSLBalances(ctx context.Context, span *trace.Span, logger libLog.Logger, parserDSL *pkgTransaction.Transaction, validate *pkgTransaction.Responses, balanceOperations []mmodel.BalanceOperation) error {
	txBalances := make([]*pkgTransaction.Balance, 0, len(balanceOperations))
	for _, bo := range balanceOperations {
		txBalances = append(txBalances, bo.Balance.ToTransactionBalance())
	}

	if err := pkgTransaction.ValidateBalancesRules(ctx, *parserDSL, *validate, txBalances); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate balances", err)

		logger.Errorf("Failed to validate balances: %v", err.Error())

		return pkg.ValidateInternalError(err, "Balance")
	}

	return nil
}

func filterBalanceOperationsForStatus(operations []mmodel.BalanceOperation, validate *pkgTransaction.Responses, transactionStatus string) []mmodel.BalanceOperation {
	if transactionStatus != constant.PENDING && transactionStatus != constant.CANCELED {
		return operations
	}

	if validate == nil || len(operations) == 0 {
		return operations
	}

	// For PENDING/CANCELED we intentionally keep only "From" entries.
	// In two-phase flows, only source-side operations affect balances (PENDING=ON_HOLD, CANCELED=RELEASE);
	// destination-side entries are no-ops for these statuses and retaining them can waste work/memory.
	capEstimate := len(validate.From)
	if capEstimate > len(operations) {
		capEstimate = len(operations)
	}
	filtered := make([]mmodel.BalanceOperation, 0, capEstimate)
	for _, op := range operations {
		if _, ok := validate.From[op.Alias]; ok {
			filtered = append(filtered, op)
		}
	}

	return filtered
}
