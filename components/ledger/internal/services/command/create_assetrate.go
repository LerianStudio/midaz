// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"

	// CreateOrUpdateAssetRate creates or updates an asset rate.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) CreateOrUpdateAssetRate(ctx context.Context, organizationID, ledgerID uuid.UUID, cari *assetrate.CreateAssetRateInput) (_ *assetrate.AssetRate, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_or_update_asset_rate")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "create_asset_rate", start, err)
	}()

	if err := utils.ValidateCode(cari.From); err != nil {
		err := pkg.ValidateBusinessError(err, constant.EntityAssetRate)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate 'from' asset code", err)

		return nil, err
	}

	if err := utils.ValidateCode(cari.To); err != nil {
		err := pkg.ValidateBusinessError(err, constant.EntityAssetRate)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate 'to' asset code", err)

		return nil, err
	}

	externalID := cari.ExternalID
	emptyExternalID := libCommons.IsNilOrEmpty(externalID)

	rate := float64(cari.Rate)
	scale := float64(cari.Scale)

	arFound, err := uc.AssetRateRepo.FindByCurrencyPair(ctx, organizationID, ledgerID, cari.From, cari.To)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to find asset rate by currency pair", err)

		logger.Log(ctx, libLog.LevelError, "Error creating asset rate", libLog.Err(err))

		return nil, err
	}

	if arFound != nil {
		arFound.Rate = rate
		arFound.Scale = &scale
		arFound.Source = cari.Source
		arFound.TTL = *cari.TTL
		arFound.UpdatedAt = time.Now()

		if !emptyExternalID {
			arFound.ExternalID = *externalID
		}

		arFound, err = uc.AssetRateRepo.Update(ctx, organizationID, ledgerID, uuid.MustParse(arFound.ID), arFound)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update asset rate", err)

			logger.Log(ctx, libLog.LevelError, "Error updating asset rate", libLog.Err(err))

			return nil, err
		}

		metadataUpdated, err := uc.UpdateTransactionMetadata(ctx, constant.EntityAssetRate, arFound.ID, cari.Metadata)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update metadata on repo by id", err)

			return nil, err
		}

		arFound.Metadata = metadataUpdated

		return arFound, nil
	}

	if emptyExternalID {
		idStr := uuid.Must(libCommons.GenerateUUIDv7()).String()
		externalID = &idStr
	}

	assetRateDB := &assetrate.AssetRate{
		ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		ExternalID:     *externalID,
		From:           cari.From,
		To:             cari.To,
		Rate:           rate,
		Scale:          &scale,
		Source:         cari.Source,
		TTL:            *cari.TTL,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	assetRate, err := uc.AssetRateRepo.Create(ctx, assetRateDB)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset rate on repository", err)

		logger.Log(ctx, libLog.LevelError, "Error creating asset rate", libLog.Err(err))

		return nil, err
	}

	if cari.Metadata != nil {
		meta := mongodb.Metadata{
			EntityID:   assetRate.ID,
			EntityName: constant.EntityAssetRate,
			Data:       cari.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.TransactionMetadataRepo.Create(ctx, constant.EntityAssetRate, &meta); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset rate metadata", err)

			logger.Log(ctx, libLog.LevelError, "Error into creating asset rate metadata", libLog.Err(err))

			return nil, err
		}

		assetRate.Metadata = cari.Metadata
	}

	return assetRate, nil
}
