package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"
)

// CreateOrUpdateAssetRate creates or updates an asset rate.
func (uc *UseCase) CreateOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID uuid.UUID, cari *mmodel.CreateAssetRateInput) (*mmodel.AssetRate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_or_update_asset_rate")
	defer span.End()

	logger.Infof("Initializing the create or update asset rate operation: %v", cari)

	if err := uc.validateAssetCodes(&span, cari); err != nil {
		return nil, err
	}

	// Validate rate is positive and within reasonable bounds
	if cari.Rate.LessThanOrEqual(decimal.Zero) {
		businessErr := pkg.ValidateBusinessError(constant.ErrInvalidRateValue, reflect.TypeOf(mmodel.AssetRate{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Rate must be positive", businessErr)

		return nil, businessErr
	}

	arFound, err := uc.findExistingAssetRate(ctx, &span, logger, organizationID, ledgerID, cari)
	if err != nil {
		return nil, err
	}

	if arFound != nil {
		return uc.updateExistingAssetRate(ctx, &span, logger, organizationID, ledgerID, cari, arFound)
	}

	return uc.createNewAssetRate(ctx, &span, logger, organizationID, ledgerID, cari)
}

// validateAssetCodes validates from and to asset codes
func (uc *UseCase) validateAssetCodes(span *trace.Span, cari *mmodel.CreateAssetRateInput) error {
	if err := utils.ValidateCode(cari.From); err != nil {
		mapped := mapCodeValidationError(err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate 'from' asset code", mapped)

		return mapped
	}

	if err := utils.ValidateCode(cari.To); err != nil {
		mapped := mapCodeValidationError(err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate 'to' asset code", mapped)

		return mapped
	}

	return nil
}

// mapCodeValidationError maps utils validation errors to business errors
func mapCodeValidationError(err error) error {
	entityType := reflect.TypeOf(mmodel.AssetRate{}).Name()

	switch err.Error() {
	case constant.ErrInvalidCodeFormat.Error():
		return pkg.ValidateBusinessError(constant.ErrInvalidCodeFormat, entityType)
	case constant.ErrCodeUppercaseRequirement.Error():
		return pkg.ValidateBusinessError(constant.ErrCodeUppercaseRequirement, entityType)
	case constant.ErrInvalidCodeLength.Error():
		return pkg.ValidateBusinessError(constant.ErrInvalidCodeLength, entityType)
	default:
		return pkg.ValidateBusinessError(err, entityType)
	}
}

// findExistingAssetRate finds an existing asset rate by currency pair
func (uc *UseCase) findExistingAssetRate(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, cari *mmodel.CreateAssetRateInput) (*mmodel.AssetRate, error) {
	logger.Infof("Trying to find existing asset rate by currency pair: %v", cari)

	arFound, err := uc.AssetRateRepo.FindByCurrencyPair(ctx, organizationID, ledgerID, cari.From, cari.To)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find asset rate by currency pair", err)
		logger.Errorf("Error creating asset rate: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AssetRate{}).Name())
	}

	return arFound, nil
}

// updateExistingAssetRate updates an existing asset rate
func (uc *UseCase) updateExistingAssetRate(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, cari *mmodel.CreateAssetRateInput, arFound *mmodel.AssetRate) (*mmodel.AssetRate, error) {
	logger.Infof("Trying to update asset rate: %v", cari)

	uc.updateAssetRateFields(arFound, cari)

	assert.That(assert.ValidUUID(arFound.ID),
		"asset rate ID must be valid UUID",
		"value", arFound.ID)

	updated, err := uc.AssetRateRepo.Update(ctx, organizationID, ledgerID, uuid.MustParse(arFound.ID), arFound)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update asset rate", err)
		logger.Errorf("Error updating asset rate: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AssetRate{}).Name())
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.AssetRate{}).Name(), updated.ID, cari.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AssetRate{}).Name())
	}

	updated.Metadata = metadataUpdated

	return updated, nil
}

// updateAssetRateFields updates asset rate fields from input
func (uc *UseCase) updateAssetRateFields(arFound *mmodel.AssetRate, cari *mmodel.CreateAssetRateInput) {
	arFound.Rate = cari.Rate
	arFound.Scale = cari.Scale
	arFound.Source = cari.Source

	if cari.TTL != nil {
		arFound.TTL = *cari.TTL
	} else {
		arFound.TTL = 0
	}

	arFound.UpdatedAt = time.Now()

	if !libCommons.IsNilOrEmpty(cari.ExternalID) {
		arFound.ExternalID = *cari.ExternalID
	}
}

// createNewAssetRate creates a new asset rate
func (uc *UseCase) createNewAssetRate(ctx context.Context, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, cari *mmodel.CreateAssetRateInput) (*mmodel.AssetRate, error) {
	logger.Infof("Trying to create asset rate: %v", cari)

	externalID := cari.ExternalID
	if libCommons.IsNilOrEmpty(externalID) {
		idStr := libCommons.GenerateUUIDv7().String()
		externalID = &idStr
	}

	var ttl int
	if cari.TTL != nil {
		ttl = *cari.TTL
	}

	assetRateDB := &mmodel.AssetRate{
		ID:             libCommons.GenerateUUIDv7().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     *externalID,
		From:           cari.From,
		To:             cari.To,
		Rate:           cari.Rate,
		Scale:          cari.Scale,
		Source:         cari.Source,
		TTL:            ttl,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	assetRate, err := uc.AssetRateRepo.Create(ctx, assetRateDB)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset rate on repository", err)
		logger.Errorf("Error creating asset rate: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AssetRate{}).Name())
	}

	if err := uc.createAssetRateMetadata(ctx, span, logger, assetRate.ID, cari.Metadata); err != nil {
		return nil, err
	}

	assetRate.Metadata = cari.Metadata

	return assetRate, nil
}

// createAssetRateMetadata creates metadata for an asset rate
func (uc *UseCase) createAssetRateMetadata(ctx context.Context, span *trace.Span, logger libLog.Logger, assetRateID string, metadata map[string]any) error {
	if metadata == nil {
		return nil
	}

	meta := mongodb.Metadata{
		EntityID:   assetRateID,
		EntityName: reflect.TypeOf(mmodel.AssetRate{}).Name(),
		Data:       metadata,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(mmodel.AssetRate{}).Name(), &meta); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset rate metadata", err)
		logger.Errorf("Error into creating asset rate metadata: %v", err)

		return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AssetRate{}).Name())
	}

	return nil
}
