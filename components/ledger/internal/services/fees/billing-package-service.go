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

	billing_package "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/billing_package"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"

	"github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// BillingPackageService handles business logic for billing package CRUD operations.
type BillingPackageService struct {
	billingPackageRepo billing_package.Repository
	midazClient        http.MidazClient
}

// ErrNilBillingPackageRepo is returned when a nil billing package repository is provided.
var ErrNilBillingPackageRepo = errors.New("BillingPackage repository is required")

// ErrNilBillingPackageMidazClient is returned when a nil MidazClient is provided.
var ErrNilBillingPackageMidazClient = errors.New("MidazClient is required")

// NewBillingPackageService creates a new BillingPackageService with validated dependencies.
func NewBillingPackageService(repo billing_package.Repository, midazClient http.MidazClient) (*BillingPackageService, error) {
	if repo == nil {
		return nil, ErrNilBillingPackageRepo
	}

	if midazClient == nil {
		return nil, ErrNilBillingPackageMidazClient
	}

	return &BillingPackageService{
		billingPackageRepo: repo,
		midazClient:        midazClient,
	}, nil
}

// CreateBillingPackage validates and creates a new billing package.
func (s *BillingPackageService) CreateBillingPackage(ctx context.Context, bp *model.BillingPackage) (*model.BillingPackage, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_package.create")
	defer span.End()

	if bp == nil {
		return nil, errors.New("billing package cannot be nil")
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", bp.OrganizationID),
		attribute.String("app.request.billing_package_type", bp.Type),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Creating billing package: org=%s, type=%s", bp.OrganizationID, bp.Type))

	// Step 1: Validate model fields (type-specific validation).
	if err := bp.Validate(); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Billing package validation failed", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Billing package validation failed: %v", err))

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
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating billing package: %v", err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Billing package created successfully: id=%s", result.ID))

	return result, nil
}

// validateVolumeCreate performs volume-specific validation: route overlap and account checks.
func (s *BillingPackageService) validateVolumeCreate(ctx context.Context, bp *model.BillingPackage) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

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
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error checking route overlap: %v", err))

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
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Billing route overlap: org=%s, route=%s", bp.OrganizationID, bp.EventFilter.TransactionRoute))

		return conflictErr
	}

	// Validate debit account exists on Midaz.
	if bp.DebitAccountAlias != nil {
		if errDebit := s.midazClient.GetAccountFromMidazByAlias(ctx, *bp.DebitAccountAlias, bp.OrganizationID, bp.LedgerID); errDebit != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(childSpan, "Debit account validation failed on Midaz", errDebit)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Debit account not found on Midaz: alias=%s, err=%v", *bp.DebitAccountAlias, errDebit))

			return errDebit
		}
	}

	// Validate credit account exists on Midaz.
	if bp.CreditAccountAlias != nil {
		if errCredit := s.midazClient.GetAccountFromMidazByAlias(ctx, *bp.CreditAccountAlias, bp.OrganizationID, bp.LedgerID); errCredit != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(childSpan, "Credit account validation failed on Midaz", errCredit)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Credit account not found on Midaz: alias=%s, err=%v", *bp.CreditAccountAlias, errCredit))

			return errCredit
		}
	}

	return nil
}

// validateMaintenanceCreate performs maintenance-specific validation: account target and credit account check.
func (s *BillingPackageService) validateMaintenanceCreate(ctx context.Context, bp *model.BillingPackage) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, childSpan := tracer.Start(ctx, "service.billing_package.validate_maintenance_create")
	defer childSpan.End()

	// Validate maintenance credit account exists on Midaz.
	if bp.MaintenanceCreditAccount != nil {
		if errCredit := s.midazClient.GetAccountFromMidazByAlias(ctx, *bp.MaintenanceCreditAccount, bp.OrganizationID, bp.LedgerID); errCredit != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(childSpan, "Maintenance credit account validation failed on Midaz", errCredit)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Maintenance credit account not found on Midaz: alias=%s, err=%v", *bp.MaintenanceCreditAccount, errCredit))

			return errCredit
		}
	}

	return nil
}

// GetBillingPackageByID retrieves a billing package by ID and organization ID.
func (s *BillingPackageService) GetBillingPackageByID(ctx context.Context, id, organizationID string) (*model.BillingPackage, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_package.get_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.billing_package_id", id),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Retrieving billing package: id=%s, org=%s", id, organizationID))

	result, err := s.billingPackageRepo.FindByID(ctx, id, organizationID)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			bizErr := pkg.ValidateBusinessError(constant.ErrBillingPackageNotFound, "BillingPackage", id)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Billing package not found", bizErr)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Billing package not found: id=%s, org=%s", id, organizationID))

			return nil, bizErr
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get billing package by id", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting billing package: id=%s, err=%v", id, err))

		return nil, err
	}

	return result, nil
}

// GetAllBillingPackages retrieves all billing packages for an organization and ledger with pagination.
func (s *BillingPackageService) GetAllBillingPackages(ctx context.Context, organizationID, ledgerID, billingType string, limit, page int) ([]*model.BillingPackage, int64, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_package.get_all")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.ledger_id", ledgerID),
		attribute.Int("app.request.limit", limit),
		attribute.Int("app.request.page", page),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Retrieving all billing packages: org=%s, ledger=%s, limit=%d, page=%d", organizationID, ledgerID, limit, page))

	results, total, err := s.billingPackageRepo.FindAll(ctx, organizationID, ledgerID, billingType, limit, page)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get all billing packages", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting all billing packages: org=%s, err=%v", organizationID, err))

		return nil, 0, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Retrieved %d billing packages (total=%d)", len(results), total))

	return results, total, nil
}

// UpdateBillingPackage updates a billing package by ID with the provided fields.
func (s *BillingPackageService) UpdateBillingPackage(ctx context.Context, id, organizationID string, updates map[string]any) (*model.BillingPackage, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_package.update")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.billing_package_id", id),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Updating billing package: id=%s, org=%s", id, organizationID))

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
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating billing package: id=%s, err=%v", id, err))

		return nil, err
	}

	// Retrieve updated entity.
	result, err := s.billingPackageRepo.FindByID(ctx, id, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve billing package after update", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error retrieving billing package after update: id=%s, err=%v", id, err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Billing package updated successfully: id=%s", id))

	return result, nil
}

// DeleteBillingPackage soft-deletes a billing package by ID and organization ID.
func (s *BillingPackageService) DeleteBillingPackage(ctx context.Context, id, organizationID string) error {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.billing_package.delete")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.billing_package_id", id),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Deleting billing package: id=%s, org=%s", id, organizationID))

	if err := s.billingPackageRepo.SoftDelete(ctx, id, organizationID); err != nil {
		// Check if the error is an entity-not-found from the repo layer and remap to FEE-0052.
		var notFoundErr pkg.EntityNotFoundError
		if errors.As(err, &notFoundErr) {
			bizErr := pkg.ValidateBusinessError(constant.ErrBillingPackageNotFound, "BillingPackage", id)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Billing package not found for deletion", bizErr)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Billing package not found for deletion: id=%s, org=%s", id, organizationID))

			return bizErr
		}

		libOpentelemetry.HandleSpanError(span, "Failed to delete billing package", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error deleting billing package: id=%s, err=%v", id, err))

		return err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Billing package deleted successfully: id=%s", id))

	return nil
}
