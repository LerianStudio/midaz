// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	midaz "github.com/LerianStudio/midaz/v3/components/ledger/internal/services/fees/midaz"
	billing_package "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/billing_package"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/fee"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
)

// BillingCalculateService handles the orchestration of billing calculations
// for volume and maintenance billing packages.
type BillingCalculateService struct {
	billingPackageRepo billing_package.Repository
	transactionCounter midaz.TransactionCounter
	accountResolver    midaz.AccountResolver
}

// ErrNilBillingCalcRepo is returned when a nil billing package repository is provided.
var ErrNilBillingCalcRepo = errors.New("BillingPackage repository is required")

// ErrNilTransactionCounter is returned when a nil TransactionCounter is provided.
var ErrNilTransactionCounter = errors.New("TransactionCounter is required")

// ErrNilAccountResolver is returned when a nil AccountResolver is provided.
var ErrNilAccountResolver = errors.New("AccountResolver is required")

// periodLayoutMonthly is the expected format for monthly billing periods ("YYYY-MM").
const periodLayoutMonthly = "2006-01"

// periodLayoutDaily is the expected format for daily billing periods ("YYYY-MM-DD").
const periodLayoutDaily = "2006-01-02"

// NewBillingCalculateService creates a new BillingCalculateService with validated dependencies.
func NewBillingCalculateService(
	repo billing_package.Repository,
	counter midaz.TransactionCounter,
	resolver midaz.AccountResolver,
) (*BillingCalculateService, error) {
	if repo == nil {
		return nil, ErrNilBillingCalcRepo
	}

	if counter == nil {
		return nil, ErrNilTransactionCounter
	}

	if resolver == nil {
		return nil, ErrNilAccountResolver
	}

	return &BillingCalculateService{
		billingPackageRepo: repo,
		transactionCounter: counter,
		accountResolver:    resolver,
	}, nil
}

// Calculate performs billing calculation for the given request. It fetches active
// billing packages by type, computes the billing result for each, and returns
// a consolidated response with one result per package.
//
// If no active packages are found, it returns an empty result set (not an error).
// If any individual package calculation fails, the entire operation returns an error
// identifying the failed package by ID and label.
func (s *BillingCalculateService) Calculate(
	ctx context.Context,
	req model.BillingCalculateRequest,
) (*model.BillingCalculateResponse, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_calculate.calculate")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", req.OrganizationID),
		attribute.String("app.request.ledger_id", req.LedgerID),
		attribute.String("app.request.period", req.Period),
		attribute.String("app.request.type", req.Type),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Starting billing calculation: org=%s, ledger=%s, period=%s, type=%s",
		req.OrganizationID, req.LedgerID, req.Period, req.Type))

	// Step 1: Validate UUIDs before any database calls to fail fast on invalid input.
	orgUUID, errOrg := uuid.Parse(req.OrganizationID)
	if errOrg != nil {
		bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", "invalid organizationID UUID")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid organization UUID", bizErr)

		return nil, bizErr
	}

	ledgerUUID, errLedger := uuid.Parse(req.LedgerID)
	if errLedger != nil {
		bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", "invalid ledgerID UUID")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ledger UUID", bizErr)

		return nil, bizErr
	}

	// Step 2: Parse and validate the period.
	periodStart, periodEnd, err := parsePeriod(req.Period)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid billing period", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid billing period: period=%s, err=%v", req.Period, err))

		return nil, err
	}

	// Step 3: Fetch active packages by type.
	volumePackages, maintenancePackages, err := s.fetchPackagesByType(ctx, req)
	if err != nil {
		return nil, err
	}

	totalPackages := len(volumePackages) + len(maintenancePackages)
	if totalPackages == 0 {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("No active billing packages found: org=%s, ledger=%s", req.OrganizationID, req.LedgerID))

		return &model.BillingCalculateResponse{
			Results: []model.BillingCalculationResult{},
			Summary: model.BillingCalculateSummary{
				TotalNetAmount: decimal.Zero,
			},
		}, nil
	}

	span.SetAttributes(
		attribute.Int("app.request.volume_packages", len(volumePackages)),
		attribute.Int("app.request.maintenance_packages", len(maintenancePackages)),
	)

	// Step 4: Process volume packages.
	results := make([]model.BillingCalculationResult, 0, totalPackages)

	for _, bp := range volumePackages {
		result, errCalc := s.calculateVolume(ctx, bp, req.Period, periodStart, periodEnd, orgUUID, ledgerUUID)
		if errCalc != nil {
			return nil, errCalc
		}

		if result != nil {
			results = append(results, *result)
		}
	}

	// Step 5: Process maintenance packages.
	for _, bp := range maintenancePackages {
		result, errCalc := s.calculateMaintenance(ctx, bp, req.Period, orgUUID, ledgerUUID)
		if errCalc != nil {
			return nil, errCalc
		}

		if result != nil {
			results = append(results, *result)
		}
	}

	// Step 6: Build summary.
	summary := buildSummary(results)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Billing calculation completed: totalResults=%d, totalVolume=%d, totalMaintenance=%d, totalNetAmount=%s",
		summary.TotalResults, summary.TotalVolume, summary.TotalMaintenance, summary.TotalNetAmount.String()))

	span.SetAttributes(
		attribute.Int("app.response.total_results", summary.TotalResults),
		attribute.String("app.response.total_net_amount", summary.TotalNetAmount.String()),
	)

	return &model.BillingCalculateResponse{
		Results: results,
		Summary: summary,
	}, nil
}

// parsePeriod parses a billing period string into start and end times.
// It accepts three formats:
//   - "YYYY-MM-DD" (daily): start is the beginning of the day; end is the beginning of the next day.
//   - "YYYY-Www" (weekly): start is Monday of the ISO week; end is the following Monday.
//   - "YYYY-MM" (monthly): start is the first instant of the month; end is the first instant of the next month.
func parsePeriod(period string) (time.Time, time.Time, error) {
	if period == "" {
		return time.Time{}, time.Time{}, pkg.ValidateBusinessError(constant.ErrInvalidBillingPeriod, "BillingCalculation", "period is required")
	}

	// Try daily format first (most specific).
	if t, err := time.Parse(periodLayoutDaily, period); err == nil {
		start := t.UTC()
		end := start.AddDate(0, 0, 1)

		return start, end, nil
	}

	// Try weekly format (YYYY-Www).
	if start, end, ok := model.ParseWeeklyPeriod(period); ok {
		return start, end, nil
	}

	// Fall back to monthly format.
	if t, err := time.Parse(periodLayoutMonthly, period); err == nil {
		start := t.UTC()
		end := start.AddDate(0, 1, 0)

		return start, end, nil
	}

	// Distinguish between a structurally valid weekly period with a non-existent ISO week
	// (e.g. 2025-W53) and a completely wrong format — so callers know the right fix.
	if model.LooksLikeWeeklyPeriod(period) {
		return time.Time{}, time.Time{}, pkg.ValidateBusinessError(constant.ErrInvalidBillingPeriod, "BillingCalculation",
			fmt.Sprintf("period %q is not a valid ISO week (week does not exist in that year)", period))
	}

	return time.Time{}, time.Time{}, pkg.ValidateBusinessError(constant.ErrInvalidBillingPeriod, "BillingCalculation",
		fmt.Sprintf("invalid format %q, expected YYYY-MM, YYYY-MM-DD, or YYYY-Www", period))
}

// fetchPackagesByType retrieves active billing packages based on the requested type.
// When type is empty, both volume and maintenance packages are fetched.
func (s *BillingCalculateService) fetchPackagesByType(
	ctx context.Context,
	req model.BillingCalculateRequest,
) ([]*model.BillingPackage, []*model.BillingPackage, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_calculate.fetch_packages")
	defer span.End()

	var volumePackages []*model.BillingPackage

	var maintenancePackages []*model.BillingPackage

	switch req.Type {
	case model.BillingPackageTypeVolume:
		pkgs, err := s.billingPackageRepo.FindActiveByType(ctx, req.OrganizationID, req.LedgerID, model.BillingPackageTypeVolume)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to fetch volume packages", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error fetching volume packages: org=%s, err=%v", req.OrganizationID, err))

			return nil, nil, err
		}

		volumePackages = pkgs

	case model.BillingPackageTypeMaintenance:
		pkgs, err := s.billingPackageRepo.FindActiveByType(ctx, req.OrganizationID, req.LedgerID, model.BillingPackageTypeMaintenance)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to fetch maintenance packages", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error fetching maintenance packages: org=%s, err=%v", req.OrganizationID, err))

			return nil, nil, err
		}

		maintenancePackages = pkgs

	default:
		// Fetch both types.
		volPkgs, errVol := s.billingPackageRepo.FindActiveByType(ctx, req.OrganizationID, req.LedgerID, model.BillingPackageTypeVolume)
		if errVol != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to fetch volume packages", errVol)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error fetching volume packages: org=%s, err=%v", req.OrganizationID, errVol))

			return nil, nil, errVol
		}

		maintPkgs, errMaint := s.billingPackageRepo.FindActiveByType(ctx, req.OrganizationID, req.LedgerID, model.BillingPackageTypeMaintenance)
		if errMaint != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to fetch maintenance packages", errMaint)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error fetching maintenance packages: org=%s, err=%v", req.OrganizationID, errMaint))

			return nil, nil, errMaint
		}

		volumePackages = volPkgs
		maintenancePackages = maintPkgs
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Fetched packages: volume=%d, maintenance=%d", len(volumePackages), len(maintenancePackages)))

	return volumePackages, maintenancePackages, nil
}

// calculateVolume processes a single volume billing package: counts transactions,
// applies free quota, calculates tiered/fixed pricing, applies discount, and builds payload.
func (s *BillingCalculateService) calculateVolume(
	ctx context.Context,
	bp *model.BillingPackage,
	period string,
	periodStart, periodEnd time.Time,
	orgUUID, ledgerUUID uuid.UUID,
) (*model.BillingCalculationResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_calculate.calculate_volume")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.billing_package_id", bp.ID),
		attribute.String("app.request.billing_package_label", bp.Label),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Calculating volume billing: packageId=%s, label=%s, period=%s", bp.ID, bp.Label, period))

	// Guard: EventFilter must be present — it is enforced at creation time but a document
	// stored without this field (e.g. from a schema migration) would panic without this check.
	if bp.EventFilter == nil {
		errMsg := fmt.Sprintf("billing package (id=%s, label=%s): missing event filter", bp.ID, bp.Label)
		bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", errMsg)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing event filter for volume package", bizErr)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Missing event filter: packageId=%s", bp.ID))

		return nil, bizErr
	}

	// Step 1: Count transactions for this route.
	countParams := midaz.CountParams{
		OrganizationID: orgUUID,
		LedgerID:       ledgerUUID,
		Route:          bp.EventFilter.TransactionRoute,
		Status:         bp.EventFilter.Status,
		StartDate:      periodStart,
		EndDate:        periodEnd,
	}

	totalEvents, err := s.transactionCounter.CountByRoute(ctx, countParams)
	if err != nil {
		errMsg := fmt.Sprintf("billing package (id=%s, label=%s): failed to count transactions: %v", bp.ID, bp.Label, err)
		bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", errMsg)

		libOpentelemetry.HandleSpanError(span, "Failed to count transactions for volume package", bizErr)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error counting transactions: packageId=%s, err=%v", bp.ID, err))

		return nil, bizErr
	}

	span.SetAttributes(attribute.Int64("app.response.total_events", totalEvents))

	// Step 2: Apply free quota.
	freeQuota := 0
	if bp.FreeQuota != nil {
		freeQuota = *bp.FreeQuota
	}

	billableEvents := fee.ApplyFreeQuota(totalEvents, freeQuota)

	// Step 3: Calculate pricing based on model.
	var unitPrice, grossAmount decimal.Decimal

	pricingModel := ""
	if bp.PricingModel != nil {
		pricingModel = *bp.PricingModel
	}

	switch pricingModel {
	case model.PricingModelTiered:
		var errTier error

		unitPrice, grossAmount, errTier = fee.CalculateTiered(billableEvents, bp.Tiers)
		if errTier != nil {
			errMsg := fmt.Sprintf("billing package (id=%s, label=%s): tiered calculation failed: %v", bp.ID, bp.Label, errTier)
			bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", errMsg)

			libOpentelemetry.HandleSpanError(span, "Tiered calculation failed", bizErr)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Tiered calculation error: packageId=%s, err=%v", bp.ID, errTier))

			return nil, bizErr
		}

	case model.PricingModelFixed:
		if len(bp.Tiers) == 0 {
			errMsg := fmt.Sprintf("billing package (id=%s, label=%s): fixed pricing requires at least one tier", bp.ID, bp.Label)
			bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", errMsg)

			libOpentelemetry.HandleSpanError(span, "Fixed pricing with no tiers", bizErr)

			return nil, bizErr
		}

		unitPrice = bp.Tiers[0].UnitPrice
		grossAmount = fee.CalculateFixed(billableEvents, unitPrice)

	default:
		errMsg := fmt.Sprintf("billing package (id=%s, label=%s): unknown pricing model: %s", bp.ID, bp.Label, pricingModel)
		bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", errMsg)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Unknown pricing model", bizErr)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Unknown pricing model: packageId=%s, model=%s", bp.ID, pricingModel))

		return nil, bizErr
	}

	// Step 4: Apply discount on the gross amount. Amounts are kept at full
	// decimal precision (the ledger is arbitrary-precision; P4-T23 found no
	// lossy serialization boundary), so no asset-scale rounding is applied here.
	netAmount, discount := fee.ApplyDiscount(grossAmount, totalEvents, bp.DiscountTiers)

	// Step 4a: Derive net = gross - discount so net is always gross minus discount.
	if discount != nil {
		netAmount = grossAmount.Sub(discount.DiscountAmount)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Volume calculation result: packageId=%s, totalEvents=%d, billable=%d, unitPrice=%s, gross=%s, net=%s",
		bp.ID, totalEvents, billableEvents, unitPrice.String(), grossAmount.String(), netAmount.String()))

	// Step 5: If net amount is zero (e.g. free quota covered all events), return result
	// with empty payload {} to signal "processed but nothing to submit to Midaz".
	if netAmount.IsZero() {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Volume billing net amount is zero (free quota or discount covered all): packageId=%s, totalEvents=%d, billable=%d, skipping payload generation",
			bp.ID, totalEvents, billableEvents))

		return &model.BillingCalculationResult{
			BillingPackageID:    bp.ID,
			BillingPackageLabel: bp.Label,
			BillingType:         model.BillingPackageTypeVolume,
			Period:              period,
			TotalAccounts:       0,
			TotalCharged:        0,
			TotalSkipped:        0,
			TotalNetAmount:      decimal.Zero,
			TransactionPayload:  json.RawMessage("{}"),
		}, nil
	}

	// Step 6: Build transaction payload.
	txPayload := BuildVolumePayload(ctx, *bp, period, totalEvents, netAmount, discount)
	if txPayload == nil {
		errMsg := fmt.Sprintf("billing package (id=%s, label=%s): volume payload builder returned nil", bp.ID, bp.Label)
		bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", errMsg)

		libOpentelemetry.HandleSpanError(span, "Volume payload builder returned nil", bizErr)

		return nil, bizErr
	}

	payloadBytes, err := json.Marshal(txPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal volume transaction payload: %w", err)
	}

	return &model.BillingCalculationResult{
		BillingPackageID:    bp.ID,
		BillingPackageLabel: bp.Label,
		BillingType:         model.BillingPackageTypeVolume,
		Period:              period,
		TotalAccounts:       0,
		TotalCharged:        0,
		TotalSkipped:        0,
		TotalNetAmount:      netAmount,
		TransactionPayload:  json.RawMessage(payloadBytes),
	}, nil
}

// calculateMaintenance processes a single maintenance billing package: resolves accounts
// and builds the N:1 transaction payload.
func (s *BillingCalculateService) calculateMaintenance(
	ctx context.Context,
	bp *model.BillingPackage,
	period string,
	orgUUID, ledgerUUID uuid.UUID,
) (*model.BillingCalculationResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_calculate.calculate_maintenance")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.billing_package_id", bp.ID),
		attribute.String("app.request.billing_package_label", bp.Label),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Calculating maintenance billing: packageId=%s, label=%s, period=%s", bp.ID, bp.Label, period))

	// Step 1: Resolve accounts.
	if bp.AccountTarget == nil {
		errMsg := fmt.Sprintf("billing package (id=%s, label=%s): missing account target", bp.ID, bp.Label)
		bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", errMsg)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing account target for maintenance package", bizErr)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Missing account target: packageId=%s", bp.ID))

		return nil, bizErr
	}

	accounts, err := s.accountResolver.ResolveAccounts(ctx, orgUUID, ledgerUUID, *bp.AccountTarget)
	if err != nil {
		errMsg := fmt.Sprintf("billing package (id=%s, label=%s): failed to resolve accounts: %v", bp.ID, bp.Label, err)
		bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", errMsg)

		libOpentelemetry.HandleSpanError(span, "Failed to resolve accounts for maintenance package", bizErr)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error resolving accounts: packageId=%s, err=%v", bp.ID, err))

		return nil, bizErr
	}

	span.SetAttributes(attribute.Int("app.response.resolved_accounts", len(accounts)))

	// Guard: skip payload generation when no accounts are resolved to avoid
	// degenerate transactions (send.value=0, from=[]) that Midaz would reject.
	if len(accounts) == 0 {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("No active accounts resolved for maintenance package: packageId=%s, skipping payload generation", bp.ID))

		return &model.BillingCalculationResult{
			BillingPackageID:    bp.ID,
			BillingPackageLabel: bp.Label,
			BillingType:         bp.Type,
			Period:              period,
			TotalAccounts:       0,
			TotalCharged:        0,
			TotalSkipped:        0,
			TotalNetAmount:      decimal.Zero,
			TransactionPayload:  json.RawMessage("{}"),
		}, nil
	}

	// Step 2: Calculate net amount = feeAmount * accountCount. Amounts are kept
	// at full decimal precision (no asset-scale rounding; P4-T23).
	feeAmount := decimal.Zero
	if bp.FeeAmount != nil {
		feeAmount = *bp.FeeAmount
	}

	netAmount := feeAmount.Mul(decimal.NewFromInt(int64(len(accounts))))

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Maintenance calculation result: packageId=%s, accounts=%d, feeAmount=%s, netAmount=%s",
		bp.ID, len(accounts), feeAmount.String(), netAmount.String()))

	// Step 3: Build transaction payload using the rounded feeAmount so that
	// each per-account from-entry and send.value reflect the asset precision.
	bpForPayload := *bp
	bpForPayload.FeeAmount = &feeAmount

	txPayload := BuildMaintenancePayload(ctx, bpForPayload, period, accounts)
	if txPayload == nil {
		errMsg := fmt.Sprintf("billing package (id=%s, label=%s): maintenance payload builder returned nil", bp.ID, bp.Label)
		bizErr := pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingCalculation", errMsg)

		libOpentelemetry.HandleSpanError(span, "Maintenance payload builder returned nil", bizErr)

		return nil, bizErr
	}

	maintenanceBytes, err := json.Marshal(txPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal maintenance transaction payload: %w", err)
	}

	return &model.BillingCalculationResult{
		BillingPackageID:    bp.ID,
		BillingPackageLabel: bp.Label,
		BillingType:         model.BillingPackageTypeMaintenance,
		Period:              period,
		TotalAccounts:       len(accounts),
		TotalCharged:        len(accounts),
		TotalSkipped:        0,
		TotalNetAmount:      netAmount,
		TransactionPayload:  json.RawMessage(maintenanceBytes),
	}, nil
}

// buildSummary aggregates totals across all billing calculation results.
func buildSummary(results []model.BillingCalculationResult) model.BillingCalculateSummary {
	summary := model.BillingCalculateSummary{
		TotalResults:   len(results),
		TotalNetAmount: decimal.Zero,
	}

	for _, r := range results {
		summary.TotalNetAmount = summary.TotalNetAmount.Add(r.TotalNetAmount)

		switch r.BillingType {
		case model.BillingPackageTypeVolume:
			summary.TotalVolume++
		case model.BillingPackageTypeMaintenance:
			summary.TotalMaintenance++
		}
	}

	return summary
}
