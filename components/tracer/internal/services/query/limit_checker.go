// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

//go:generate mockgen -source=limit_checker.go -destination=limit_checker_mock.go -package=query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// calculateCounterExpiresAt calculates when a usage counter should expire based on limit type.
// Returns nil for PER_TRANSACTION (no counter created) or when required dates are nil.
// For DAILY/WEEKLY/MONTHLY: returns resetAt + CounterRetentionDays retention period.
// For CUSTOM: returns customEndDate + CounterRetentionDays retention period.
func calculateCounterExpiresAt(limitType model.LimitType, resetAt *time.Time, customEndDate *time.Time) *time.Time {
	switch limitType {
	case model.LimitTypeDaily, model.LimitTypeWeekly, model.LimitTypeMonthly:
		if resetAt == nil {
			return nil
		}

		exp := resetAt.AddDate(0, 0, constant.CounterRetentionDays)

		return &exp

	case model.LimitTypeCustom:
		if customEndDate == nil {
			return nil
		}

		exp := customEndDate.AddDate(0, 0, constant.CounterRetentionDays)

		return &exp

	case model.LimitTypePerTransaction:
		// PER_TRANSACTION limits don't create counters
		return nil

	default:
		return nil
	}
}

// LimitChecker defines the interface for checking limits against transactions.
type LimitChecker interface {
	// CheckLimits evaluates all applicable limits for a transaction using
	// the provided database connection. This allows callers to pass either a
	// regular DB connection or a transaction (*sql.Tx), enabling atomic operations
	// with other database changes.
	// Uses atomic upsert to prevent TOCTOU race conditions.
	//
	// Returns CheckLimitsOutput with:
	//   - Allowed: true if no limits exceeded (or no active limits found)
	//   - ExceededLimitIDs: IDs of limits that would be exceeded
	//   - LimitUsageDetails: usage information for all checked limits
	//
	// For DAILY/MONTHLY limits: increments usage counters atomically
	// For PER_TRANSACTION limits: checks maxAmount directly without persistent counters
	//
	// When db is provided (non-nil):
	//   - Uses UpsertAndIncrementAtomic and GetUsageForLimits for transactional operations
	//   - Does NOT perform compensating rollback on limit exceeded - caller MUST call tx.Rollback()
	//   - This enables the caller to atomically rollback ALL changes (counters, validation, audit)
	CheckLimits(ctx context.Context, db pgdb.DB, input *model.CheckLimitsInput) (*model.CheckLimitsOutput, error)
}

// LimitCheckerService implements LimitChecker using repository pattern.
type LimitCheckerService struct {
	limitRepo        LimitRepository
	usageCounterRepo UsageCounterRepository
	clock            clock.Clock
}

// NewLimitChecker creates a new LimitCheckerService.
// Returns error if any dependency is nil.
func NewLimitChecker(limitRepo LimitRepository, usageCounterRepo UsageCounterRepository, clk clock.Clock) (*LimitCheckerService, error) {
	if limitRepo == nil {
		return nil, constant.ErrLimitCheckerNilLimitRepo
	}

	if usageCounterRepo == nil {
		return nil, constant.ErrLimitCheckerNilUsageCounterRepo
	}

	if clk == nil {
		return nil, constant.ErrLimitCheckerNilClock
	}

	return &LimitCheckerService{
		limitRepo:        limitRepo,
		usageCounterRepo: usageCounterRepo,
		clock:            clk,
	}, nil
}

// CheckLimits evaluates all applicable limits for a transaction using the provided database connection.
// This allows callers to pass either a regular DB connection or a transaction (*sql.Tx),
// enabling atomic operations with other database changes.
// Uses atomic upsert to prevent TOCTOU race conditions.
//
// The db parameter MUST be non-nil:
//   - Uses UpsertAndIncrementAtomic and GetUsageForLimits for transactional operations
//   - Does NOT perform compensating rollback on limit exceeded - caller MUST call tx.Rollback()
//   - This enables the caller to atomically rollback ALL changes (counters, validation, audit)
func (s *LimitCheckerService) CheckLimits(ctx context.Context, db pgdb.DB, input *model.CheckLimitsInput) (*model.CheckLimitsOutput, error) {
	if db == nil {
		return nil, pgdb.ErrNilConnection
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.limit_checker.check_limits")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Pass db to enable transactional operations
	return s.checkLimitsInternal(ctx, db, input, logger, span, "service.limit_checker.check_limits")
}

// checkLimitsInternal contains the core logic for CheckLimits.
// It validates input, retrieves applicable limits, and processes each limit atomically.
//
// Transactional mode (db is always non-nil):
//   - Uses transactional repository methods (UpsertAndIncrementAtomic, GetUsageForLimits)
//   - Does NOT perform compensating rollback on limit exceeded or error
//   - Caller is responsible for tx.Rollback() to atomically undo ALL changes
func (s *LimitCheckerService) checkLimitsInternal(
	ctx context.Context,
	db pgdb.DB,
	input *model.CheckLimitsInput,
	logger libLog.Logger,
	span trace.Span,
	operationName string,
) (*model.CheckLimitsOutput, error) {
	if input == nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Nil input", constant.ErrCheckLimitsNilInput)
		return nil, constant.ErrCheckLimitsNilInput
	}

	if err := input.Validate(); err != nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Invalid input", err)
		return nil, err
	}

	if err := libOtel.SetSpanAttributesFromValue(span, "input", input, nil); err != nil {
		span.RecordError(err)

		logger.With(
			libLog.String("operation", operationName),
			libLog.String("error", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Failed to set span attributes for input")
	}

	// Get applicable limits (active limits matching currency and scopes)
	limits, err := s.getApplicableLimits(ctx, input)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get applicable limits", err)
		return nil, err
	}

	// Compute server timestamp once for consistent evaluatedAt across all paths
	serverNow := s.clock.Now()

	if len(limits) == 0 {
		logger.With(
			libLog.String("operation", operationName),
			libLog.String("currency", input.Currency),
		).Log(ctx, libLog.LevelInfo, "No active limits found for criteria")

		output := model.NewCheckLimitsOutput(true, serverNow)

		return output, nil
	}

	logger.With(
		libLog.String("operation", operationName),
		libLog.Int("applicable_limits_count", len(limits)),
	).Log(ctx, libLog.LevelInfo, "Found applicable limits")

	// Build transaction scope once for all limits
	txScope := buildTransactionScope(input)

	// Process each limit with atomic upsert (increment happens in DB)
	usageDetails := make([]model.LimitUsageDetail, 0, len(limits))

	var exceededLimitIDs []uuid.UUID

	for i := range limits {
		limit := &limits[i]

		// Calculate scope key based on the limit's scope, not transaction's full scope.
		// This prevents counter fragmentation when limits have different scope granularities.
		// Example: account-only limit must use "acct:X" key, not "acct:X:seg:Y:port:Z".
		scopeKey := calculateScopeKeyFromScopes(limit.Scopes, txScope)

		detail, exceeded, err := s.processLimitAtomic(ctx, db, limit, input, scopeKey, serverNow)
		if err != nil {
			// DB error - caller will do tx.Rollback() to atomically undo all changes
			libOtel.HandleSpanError(span, "Failed to process limit atomically", err)
			return nil, err
		}

		usageDetails = append(usageDetails, *detail)

		if exceeded {
			exceededLimitIDs = append(exceededLimitIDs, limit.ID)
		}
	}

	allowed := len(exceededLimitIDs) == 0
	output := model.NewCheckLimitsOutput(allowed, serverNow).WithLimitUsageDetails(usageDetails)

	if !allowed {
		output = output.WithExceededLimits(exceededLimitIDs)

		logger.With(
			libLog.String("operation", operationName),
			libLog.Any("exceeded_limit_ids", exceededLimitIDs),
			libLog.Int("exceeded_count", len(exceededLimitIDs)),
		).Log(ctx, libLog.LevelInfo, "Limits exceeded")
	} else {
		logger.With(
			libLog.String("operation", operationName),
			libLog.Int("checked_count", len(usageDetails)),
		).Log(ctx, libLog.LevelInfo, "All limits passed")
	}

	return output, nil
}

// skipIfOutsideTimeWindow checks if transaction is outside the limit's time window.
// Returns (limitUsageDetail, true) if should skip, (nil, false) if should process.
// SECURITY: Uses server time (not client timestamp) to prevent timestamp injection attacks.
func skipIfOutsideTimeWindow(
	limit *model.Limit,
	input *model.CheckLimitsInput,
	serverNow time.Time,
) (*model.LimitUsageDetail, bool) {
	if !limit.IsWithinTimeWindow(serverNow) {
		// Debug only - hot path logging removed for performance
		// Time window skips are normal behavior, not errors
		return &model.LimitUsageDetail{
			LimitID:           limit.ID,
			LimitAmount:       limit.MaxAmount,
			Scope:             formatScopeString(limit.Scopes),
			Period:            limit.LimitType,
			CurrentUsage:      decimal.Zero,
			AttemptedAmount:   input.Amount,
			Exceeded:          false,
			Skipped:           true,
			SkipReason:        "outside_time_window",
			InternalLimitType: limit.LimitType,
			Scopes:            append([]model.Scope(nil), limit.Scopes...),
		}, true
	}

	return nil, false
}

// skipIfOutsideCustomPeriod checks if transaction is outside the limit's custom period.
// Returns (limitUsageDetail, true) if should skip, (nil, false) if should process.
// SECURITY: Uses server time (not client timestamp) to prevent timestamp injection attacks.
func skipIfOutsideCustomPeriod(
	limit *model.Limit,
	input *model.CheckLimitsInput,
	serverNow time.Time,
) (*model.LimitUsageDetail, bool) {
	if !limit.IsWithinCustomPeriod(serverNow) {
		// Debug only - hot path logging removed for performance
		// Custom period skips are normal behavior, not errors
		return &model.LimitUsageDetail{
			LimitID:           limit.ID,
			LimitAmount:       limit.MaxAmount,
			Scope:             formatScopeString(limit.Scopes),
			Period:            limit.LimitType,
			CurrentUsage:      decimal.Zero,
			AttemptedAmount:   input.Amount,
			Exceeded:          false,
			Skipped:           true,
			SkipReason:        "outside_custom_period",
			InternalLimitType: limit.LimitType,
			Scopes:            append([]model.Scope(nil), limit.Scopes...),
		}, true
	}

	return nil, false
}

// handlePerTransactionLimit processes PER_TRANSACTION limits (no counter needed).
// Returns (limitUsageDetail, exceeded) for the limit check result.
func handlePerTransactionLimit(
	limit *model.Limit,
	input *model.CheckLimitsInput,
) (*model.LimitUsageDetail, bool) {
	exceeded := input.Amount.GreaterThan(limit.MaxAmount)

	detail := &model.LimitUsageDetail{
		LimitID:           limit.ID,
		LimitAmount:       limit.MaxAmount,
		Scope:             formatScopeString(limit.Scopes),
		Period:            limit.LimitType,
		CurrentUsage:      decimal.Zero, // PER_TRANSACTION has no persistent usage
		AttemptedAmount:   input.Amount,
		Exceeded:          exceeded,
		InternalLimitType: limit.LimitType,
		Scopes:            append([]model.Scope(nil), limit.Scopes...),
		InternalPeriodKey: "", // PER_TRANSACTION has no period key
	}

	// Debug only - hot path logging removed for performance
	// PER_TRANSACTION checks are high-frequency operations

	return detail, exceeded
}

// processLimitAtomic processes a single limit using atomic upsert for DAILY/MONTHLY limits.
// Returns the usage detail, whether the limit was exceeded, and any error.
//
// Transactional mode (db is always non-nil):
//   - Uses UpsertAndIncrementAtomic and GetUsageForLimits
func (s *LimitCheckerService) processLimitAtomic(
	ctx context.Context,
	db pgdb.DB,
	limit *model.Limit,
	input *model.CheckLimitsInput,
	scopeKey string,
	serverNow time.Time,
) (*model.LimitUsageDetail, bool, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.limit_checker.process_limit_atomic")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if err := libOtel.SetSpanAttributesFromValue(span, "limit", map[string]any{
		"id":        limit.ID.String(),
		"name":      limit.Name,
		"type":      string(limit.LimitType),
		"maxAmount": limit.MaxAmount,
	}, nil); err != nil {
		span.RecordError(err)

		logger.With(
			libLog.String("operation", "service.limit_checker.process_limit_atomic"),
			libLog.String("limit_id", limit.ID.String()),
			libLog.String("limit_name", limit.Name),
			libLog.String("error", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Failed to set span attributes for limit")
	}

	// Check time window FIRST (before any counter operations)
	if detail, shouldSkip := skipIfOutsideTimeWindow(limit, input, serverNow); shouldSkip {
		return detail, false, nil
	}

	// Check custom period (after time window check)
	if detail, shouldSkip := skipIfOutsideCustomPeriod(limit, input, serverNow); shouldSkip {
		return detail, false, nil
	}

	// For PER_TRANSACTION limits, check directly against maxAmount (no counter needed)
	if limit.LimitType == model.LimitTypePerTransaction {
		detail, exceeded := handlePerTransactionLimit(limit, input)
		return detail, exceeded, nil
	}

	// For DAILY/WEEKLY/MONTHLY/CUSTOM limits, use atomic upsert
	periodKey, err := model.CalculatePeriodKey(limit.LimitType, serverNow)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to calculate period key", err)
		return nil, false, err
	}

	// Calculate counter expiration time for cleanup
	resetAt := model.CalculateResetAt(limit.LimitType, serverNow)
	expiresAt := calculateCounterExpiresAt(limit.LimitType, resetAt, limit.CustomEndDate)

	// Pre-check: amount > maxAmount would always fail (INSERT path has no WHERE guard)
	if input.Amount.GreaterThan(limit.MaxAmount) {
		// Fetch current usage to report projected total accurately
		currentUsage := decimal.Zero

		usageMap, err := s.usageCounterRepo.GetUsageForLimits(ctx, db, []uuid.UUID{limit.ID}, scopeKey, periodKey)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to get existing usage for pre-check", err)
			return nil, false, fmt.Errorf("failed to get existing usage for pre-check: %w", err)
		}

		if usage, found := usageMap[limit.ID]; found {
			currentUsage = usage
		}

		detail := &model.LimitUsageDetail{
			LimitID:           limit.ID,
			LimitAmount:       limit.MaxAmount,
			Scope:             formatScopeString(limit.Scopes),
			Period:            limit.LimitType,
			CurrentUsage:      currentUsage.Add(input.Amount), // Projected total: existing + attempted
			AttemptedAmount:   input.Amount,
			Exceeded:          true,
			InternalLimitType: limit.LimitType,
			Scopes:            append([]model.Scope(nil), limit.Scopes...),
			InternalPeriodKey: periodKey,
		}

		// Debug only - pre-check logging removed for performance
		return detail, true, nil
	}

	// Atomic upsert: create or increment counter, enforcing maxAmount in DB
	newUsage, err := s.usageCounterRepo.UpsertAndIncrementAtomic(
		ctx,
		db,
		limit.ID,
		scopeKey,
		periodKey,
		input.Amount,
		limit.MaxAmount,
		expiresAt,
	)

	if errors.Is(err, constant.ErrUsageCounterExceedsLimit) {
		// Limit exceeded - the counter was NOT incremented
		libOtel.HandleSpanBusinessErrorEvent(span, "Limit exceeded", err)

		detail := &model.LimitUsageDetail{
			LimitID:           limit.ID,
			LimitAmount:       limit.MaxAmount,
			Scope:             formatScopeString(limit.Scopes),
			Period:            limit.LimitType,
			CurrentUsage:      newUsage.Add(input.Amount), // Projected usage (what it would be)
			AttemptedAmount:   input.Amount,
			Exceeded:          true,
			InternalLimitType: limit.LimitType,
			Scopes:            append([]model.Scope(nil), limit.Scopes...),
			InternalPeriodKey: periodKey,
		}

		logger.With(
			libLog.String("operation", "service.limit_checker.process_limit_atomic"),
			libLog.String("limit_id", limit.ID.String()),
			libLog.String("limit_type", string(limit.LimitType)),
			libLog.String("max_amount", limit.MaxAmount.String()),
			libLog.String("transaction_amount", input.Amount.String()),
			libLog.Bool("exceeded", true),
		).Log(ctx, libLog.LevelInfo, "Limit exceeded (atomic check)")

		return detail, true, nil
	}

	if err != nil {
		// DB error
		libOtel.HandleSpanError(span, "Failed to upsert and increment counter", err)
		return nil, false, fmt.Errorf("failed to upsert and increment counter: %w", err)
	}

	// Success: counter was incremented
	detail := &model.LimitUsageDetail{
		LimitID:           limit.ID,
		LimitAmount:       limit.MaxAmount,
		Scope:             formatScopeString(limit.Scopes),
		Period:            limit.LimitType,
		CurrentUsage:      newUsage, // Actual new usage from DB
		AttemptedAmount:   input.Amount,
		Exceeded:          false,
		InternalLimitType: limit.LimitType,
		Scopes:            append([]model.Scope(nil), limit.Scopes...),
		InternalPeriodKey: periodKey,
	}

	// Debug only - hot path success logging removed for performance
	// Only log errors/exceeded, not every successful check
	return detail, false, nil
}

// getApplicableLimits fetches active limits matching currency and scopes.
// Handles pagination to retrieve all matching limits beyond MaxPaginationLimit.
func (s *LimitCheckerService) getApplicableLimits(ctx context.Context, input *model.CheckLimitsInput) ([]model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.limit_checker.get_applicable_limits")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Fetch active limits matching currency (filtered at DB level)
	// Use pagination loop to handle cases where more limits exist than MaxPaginationLimit
	status := model.LimitStatusActive
	currency := input.Currency

	var allLimits []model.Limit

	var cursor string

	for {
		filter := &model.ListLimitsFilter{
			Status:   &status,
			Currency: &currency,
			Limit:    constant.MaxPaginationLimit,
			Cursor:   cursor,
		}

		result, err := s.limitRepo.List(ctx, filter)
		if err != nil {
			libOtel.HandleSpanError(span, "Failed to list limits", err)
			return nil, err
		}

		allLimits = append(allLimits, result.Limits...)

		// Break if no more pages
		if !result.HasMore || result.NextCursor == "" {
			break
		}

		cursor = result.NextCursor
	}

	// Filter by scope matching (scopes require in-memory evaluation)
	applicable := make([]model.Limit, 0, len(allLimits))

	// Build transaction scope from input for matching
	txScope := buildTransactionScope(input)

	for _, limit := range allLimits {
		// Scopes must match (ANY limit scope matches the transaction scope)
		if !scopeMatchesLimit(limit.Scopes, txScope) {
			continue
		}

		applicable = append(applicable, limit)
	}

	logger.With(
		libLog.String("operation", "service.limit_checker.get_applicable_limits"),
		libLog.Int("limits_for_currency", len(allLimits)),
		libLog.Int("applicable_limits", len(applicable)),
		libLog.String("currency", input.Currency),
	).Log(ctx, libLog.LevelInfo, "Filtered applicable limits")

	return applicable, nil
}

// buildTransactionScope creates a Scope from CheckLimitsInput fields.
// This scope is used for matching against limit scopes and for scopeKey generation.
func buildTransactionScope(input *model.CheckLimitsInput) *model.Scope {
	if input == nil {
		return nil
	}

	return &model.Scope{
		AccountID:       &input.AccountID,
		SegmentID:       input.SegmentID,
		PortfolioID:     input.PortfolioID,
		MerchantID:      input.MerchantID,
		TransactionType: input.TransactionType,
		SubType:         input.SubType,
	}
}

// scopeMatchesLimit checks if any of the limit's scopes match the transaction scope.
// Global limits (empty scopes) match all transactions.
func scopeMatchesLimit(limitScopes []model.Scope, txScope *model.Scope) bool {
	// Global limit (empty scopes) matches all transactions
	if len(limitScopes) == 0 {
		return true
	}

	if txScope == nil {
		return false
	}

	// Check if ANY limit scope matches the transaction scope
	for i := range limitScopes {
		if limitScopes[i].Matches(txScope) {
			return true
		}
	}

	return false
}

// calculateScopeKeyFromScopes computes the scope key from a list of scopes based on the limit's scope, not the transaction's.
// This prevents counter fragmentation when limits have different scope granularities.
// Used for both CheckLimits and rollback operations.
// Returns the first matching scope's key, or constant.GlobalScopeKey if no scopes.
func calculateScopeKeyFromScopes(scopes []model.Scope, txScope *model.Scope) string {
	if len(scopes) == 0 {
		return constant.GlobalScopeKey
	}

	// Find the first scope that matches the transaction
	// Use that scope (not the transaction scope) to calculate the key
	for i := range scopes {
		if scopes[i].Matches(txScope) {
			// Calculate key from the matched scope, not the transaction's
			return model.CalculateScopeKey(&scopes[i])
		}
	}

	// Should never reach here - scopes were already filtered as applicable
	// But defensively return a key based on transaction scope
	return model.CalculateScopeKey(txScope)
}

// formatScopeString creates a human-readable string representation of scopes.
// Format examples: "global", "(account:uuid)", "(account:uuid,segment:uuid)", "(account:a,segment:b) OR (account:c)"
// Per API Design v1.3.2 section 4.1.1 LimitUsage.scope field.
// Each scope is wrapped in parentheses; multiple scopes (OR alternatives) are joined with " OR ".
func formatScopeString(scopes []model.Scope) string {
	if len(scopes) == 0 {
		return constant.GlobalScopeKey
	}

	var scopeGroups []string

	for _, scope := range scopes {
		var fields []string

		if scope.AccountID != nil {
			fields = append(fields, "account:"+scope.AccountID.String())
		}

		if scope.SegmentID != nil {
			fields = append(fields, "segment:"+scope.SegmentID.String())
		}

		if scope.PortfolioID != nil {
			fields = append(fields, "portfolio:"+scope.PortfolioID.String())
		}

		if scope.MerchantID != nil {
			fields = append(fields, "merchant:"+scope.MerchantID.String())
		}

		if scope.TransactionType != nil {
			fields = append(fields, "transactionType:"+string(*scope.TransactionType))
		}

		if scope.SubType != nil {
			fields = append(fields, "subType:"+*scope.SubType)
		}

		if len(fields) > 0 {
			scopeGroups = append(scopeGroups, "("+strings.Join(fields, ",")+")")
		}
	}

	if len(scopeGroups) == 0 {
		return constant.GlobalScopeKey
	}

	return strings.Join(scopeGroups, " OR ")
}
