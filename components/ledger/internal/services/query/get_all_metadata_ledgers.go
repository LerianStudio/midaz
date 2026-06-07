// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataLedgers fetches all ledgers from the repository.
func (uc *UseCase) GetAllMetadataLedgers(ctx context.Context, organizationID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Ledger, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_ledgers")
	defer span.End()

	metadata, err := uc.OnboardingMetadataRepo.FindList(ctx, constant.EntityLedger, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get metadata on repo", err)
		logger.Log(ctx, libLog.LevelError, "Error getting metadata on repo")

		return nil, err
	}

	if len(metadata) == 0 {
		err := pkg.ValidateBusinessError(constant.ErrNoLedgersFound, constant.EntityLedger)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No metadata found", err)

		logger.Log(ctx, libLog.LevelWarn, "No metadata found")

		return nil, err
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	filter.EntityIDs = uuids

	ledgers, err := uc.LedgerRepo.FindAll(ctx, organizationID, filter)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoLedgersFound, constant.EntityLedger)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get ledgers on repo", err)

			logger.Log(ctx, libLog.LevelWarn, "No ledgers found")

			return nil, err
		}

		logger.Log(ctx, libLog.LevelError, "Error getting ledgers on repo")

		libOpentelemetry.HandleSpanError(span, "Failed to get ledgers on repo", err)

		return nil, err
	}

	for i := range ledgers {
		if data, ok := metadataMap[ledgers[i].ID]; ok {
			ledgers[i].Metadata = data
		}
	}

	return ledgers, nil
}
