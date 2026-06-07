// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"

	billing_package "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/billing_package"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/LerianStudio/lib-observability/metrics"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// BillingPackageService handles business logic for billing package CRUD operations.
type BillingPackageService struct {
	billingPackageRepo billing_package.Repository
	resolver           feeshared.MidazResolver

	// MetricsFactory emits the bounded domain_operations_total /
	// domain_operation_duration_ms metrics for every state-mutating billing
	// package entrypoint via utils.RecordDomainOperation. Assigned at bootstrap;
	// a nil value is a no-op so the binary runs with telemetry disabled.
	MetricsFactory *metrics.MetricsFactory
}

// ErrNilBillingPackageRepo is returned when a nil billing package repository is provided.
var ErrNilBillingPackageRepo = errors.New("BillingPackage repository is required")

// ErrNilBillingPackageResolver is returned when a nil MidazResolver is provided.
var ErrNilBillingPackageResolver = errors.New("MidazResolver is required")

// NewBillingPackageService creates a new BillingPackageService with validated dependencies.
func NewBillingPackageService(repo billing_package.Repository, resolver feeshared.MidazResolver) (*BillingPackageService, error) {
	if repo == nil {
		return nil, ErrNilBillingPackageRepo
	}

	if resolver == nil {
		return nil, ErrNilBillingPackageResolver
	}

	return &BillingPackageService{
		billingPackageRepo: repo,
		resolver:           resolver,
	}, nil
}

// resolveAccountExists validates that an account with the given alias exists,
// parsing the package's string-typed org/ledger IDs into UUIDs for the resolver.
func (s *BillingPackageService) resolveAccountExists(ctx context.Context, organizationID, ledgerID, alias string) error {
	orgUUID, err := uuid.Parse(organizationID)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingPackage", "invalid organizationID UUID")
	}

	ledgerUUID, err := uuid.Parse(ledgerID)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrBillingCalculationFailed, "BillingPackage", "invalid ledgerID UUID")
	}

	return s.resolver.AccountExistsByAlias(ctx, orgUUID, ledgerUUID, alias)
}

// CreateBillingPackage validates and creates a new billing package.
func (s *BillingPackageService) CreateBillingPackage(ctx context.Context, bp *model.BillingPackage) (_ *model.BillingPackage, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_package.create")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, s.MetricsFactory, logger, "fees", "create_billing_package", start, err)
	}()

	if bp == nil {
		return nil, errors.New("billing package cannot be nil")
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", bp.OrganizationID),
		attribute.String("app.request.billing_package_type", bp.Type),
	)

	// Step 1: Validate model fields (type-specific validation).
	if err := bp.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Billing package validation failed", err)

		return nil, err
	}

	// Step 2: Type-specific business rules.
	switch bp.Type {
	case model.BillingPackageTypeVolume:
		if err := s.validateVolumeCreate(ctx, bp); err != nil {
			return nil, err
		}
	case model.BillingPackageTypeMaintenance:
		if err := s.validateMaintenanceCreate(ctx, bp); err != nil {
			return nil, err
		}
	}

	// Step 3: Set defaults and metadata.
	now := time.Now().UTC().Format(time.RFC3339)

	bpUUID, errUUID := commons.GenerateUUIDv7()
	if errUUID != nil {
		return nil, fmt.Errorf("failed to generate UUID: %w", errUUID)
	}

	bp.ID = bpUUID.String()
	bp.CreatedAt = now
	bp.UpdatedAt = now

	// Default enable to true when the caller does not explicitly set it.
	if bp.Enable == nil {
		defaultEnable := true
		bp.Enable = &defaultEnable
	}

	span.SetAttributes(attribute.String("app.request.billing_package_id", bp.ID))

	// Step 4: Persist.
	result, err := s.billingPackageRepo.Create(ctx, bp)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create billing package on repo", err)

		return nil, err
	}

	return result, nil
}

// validateVolumeCreate performs volume-specific validation: route overlap and account checks.
func (s *BillingPackageService) validateVolumeCreate(ctx context.Context, bp *model.BillingPackage) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, childSpan := tracer.Start(ctx, "service.billing_package.validate_volume_create")
	defer childSpan.End()

	// Guard: EventFilter must be non-nil before accessing fields.
	// Although bp.Validate() checks this, validateVolumeCreate may be called independently.
	if bp.EventFilter == nil {
		return pkg.ValidateBusinessError(constant.ErrMissingVolumeFields, "BillingPackage")
	}

	// Check route overlap.
	existing, err := s.billingPackageRepo.FindMatchingPackages(
		ctx, bp.OrganizationID, bp.LedgerID, bp.EventFilter.TransactionRoute,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(childSpan, "Failed to find matching packages for route overlap check", err)

		return err
	}

	if len(existing) > 0 {
		conflictErr := pkg.EntityConflictError{
			EntityType: "BillingPackage",
			Code:       constant.ErrBillingRouteOverlap.Error(),
			Title:      "Billing route overlap",
			Message:    "A billing package already exists for this organization, ledger, and transaction route combination.",
		}
		libOpentelemetry.HandleSpanBusinessErrorEvent(childSpan, "Billing route overlap detected", conflictErr)

		return conflictErr
	}

	// Validate debit account exists.
	if bp.DebitAccountAlias != nil {
		if errDebit := s.resolveAccountExists(ctx, bp.OrganizationID, bp.LedgerID, *bp.DebitAccountAlias); errDebit != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(childSpan, "Debit account validation failed on Midaz", errDebit)

			return errDebit
		}
	}

	// Validate credit account exists.
	if bp.CreditAccountAlias != nil {
		if errCredit := s.resolveAccountExists(ctx, bp.OrganizationID, bp.LedgerID, *bp.CreditAccountAlias); errCredit != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(childSpan, "Credit account validation failed on Midaz", errCredit)

			return errCredit
		}
	}

	return nil
}

// validateMaintenanceCreate performs maintenance-specific validation: account target and credit account check.
func (s *BillingPackageService) validateMaintenanceCreate(ctx context.Context, bp *model.BillingPackage) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, childSpan := tracer.Start(ctx, "service.billing_package.validate_maintenance_create")
	defer childSpan.End()

	// Validate maintenance credit account exists.
	if bp.MaintenanceCreditAccount != nil {
		if errCredit := s.resolveAccountExists(ctx, bp.OrganizationID, bp.LedgerID, *bp.MaintenanceCreditAccount); errCredit != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(childSpan, "Maintenance credit account validation failed on Midaz", errCredit)

			return errCredit
		}
	}

	return nil
}

// GetBillingPackageByID retrieves a billing package by ID and organization ID.
func (s *BillingPackageService) GetBillingPackageByID(ctx context.Context, id, organizationID string) (*model.BillingPackage, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_package.get_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.billing_package_id", id),
	)

	result, err := s.billingPackageRepo.FindByID(ctx, id, organizationID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			bizErr := pkg.ValidateBusinessError(constant.ErrBillingPackageNotFound, "BillingPackage", id)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Billing package not found", bizErr)

			return nil, bizErr
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get billing package by id", err)

		return nil, err
	}

	return result, nil
}

// GetAllBillingPackages retrieves all billing packages for an organization and ledger with pagination.
func (s *BillingPackageService) GetAllBillingPackages(ctx context.Context, organizationID, ledgerID, billingType string, limit, page int) ([]*model.BillingPackage, int64, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_package.get_all")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.ledger_id", ledgerID),
		attribute.Int("app.request.limit", limit),
		attribute.Int("app.request.page", page),
	)

	results, total, err := s.billingPackageRepo.FindAll(ctx, organizationID, ledgerID, billingType, limit, page)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get all billing packages", err)

		return nil, 0, err
	}

	span.SetAttributes(attribute.Int64("app.response.billing_packages_total", total))

	return results, total, nil
}

// UpdateBillingPackage updates a billing package by ID with the provided fields.
func (s *BillingPackageService) UpdateBillingPackage(ctx context.Context, id, organizationID string, updates map[string]any) (_ *model.BillingPackage, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_package.update")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, s.MetricsFactory, logger, "fees", "update_billing_package", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.billing_package_id", id),
	)

	// Build bson.M from updates and add updated_at timestamp.
	setFields := bson.M{}
	for k, v := range updates {
		setFields[k] = v
	}

	setFields["updated_at"] = time.Now().UTC().Format(time.RFC3339)

	updateFields := bson.M{
		"$set": setFields,
	}

	if err := s.billingPackageRepo.Update(ctx, id, organizationID, &updateFields); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update billing package", err)

		return nil, err
	}

	// Retrieve updated entity.
	result, err := s.billingPackageRepo.FindByID(ctx, id, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve billing package after update", err)

		return nil, err
	}

	return result, nil
}

// DeleteBillingPackage soft-deletes a billing package by ID and organization ID.
func (s *BillingPackageService) DeleteBillingPackage(ctx context.Context, id, organizationID string) (err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_package.delete")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, s.MetricsFactory, logger, "fees", "delete_billing_package", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.billing_package_id", id),
	)

	if err := s.billingPackageRepo.SoftDelete(ctx, id, organizationID); err != nil {
		// Remap a repo-layer entity-not-found to the billing-package-not-found business error.
		var notFoundErr pkg.EntityNotFoundError
		if errors.As(err, &notFoundErr) {
			bizErr := pkg.ValidateBusinessError(constant.ErrBillingPackageNotFound, "BillingPackage", id)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Billing package not found for deletion", bizErr)

			return bizErr
		}

		libOpentelemetry.HandleSpanError(span, "Failed to delete billing package", err)

		return err
	}

	return nil
}
