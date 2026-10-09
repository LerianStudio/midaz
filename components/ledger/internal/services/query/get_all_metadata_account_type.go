// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// GetAllMetadataAccountType fetches the cursor page of account types of the ledger whose metadata
// matches the filter. No match yields an empty, non-nil page and an empty cursor.
func (uc *UseCase) GetAllMetadataAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.AccountType, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_account_type")
	defer span.End()

	// The cursor page is cut by PostgreSQL, which applies the ledger scope, over the full match set: a
	// cursor cannot address a metadata-store page.
	metadata, err := uc.OnboardingMetadataRepo.FindList(ctx, constant.EntityAccountType, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get metadata on repo", err)
		logger.Log(ctx, libLog.LevelError, "Error getting account type metadata on repo", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(metadata) == 0 {
		span.AddEvent("No metadata matched the filter")

		return []*mmodel.AccountType{}, libHTTP.CursorPagination{}, nil
	}

	uuids := make([]uuid.UUID, 0, len(metadata))
	invalidIDs := 0

	for _, meta := range metadata {
		id, parseErr := uuid.Parse(meta.EntityID)
		if parseErr != nil {
			invalidIDs++

			continue
		}

		uuids = append(uuids, id)
	}

	if invalidIDs > 0 {
		logger.Log(ctx, libLog.LevelWarn, "Skipped metadata entity ids that are not UUIDs",
			libLog.String("entity_name", constant.EntityAccountType), libLog.Int("skipped_count", invalidIDs))
	}

	if len(uuids) == 0 {
		return []*mmodel.AccountType{}, libHTTP.CursorPagination{}, nil
	}

	filter.EntityIDs = uuids

	accountTypes, cur, err := uc.AccountTypeRepo.FindAll(ctx, organizationID, ledgerID, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get account types on repo", err)
		logger.Log(ctx, libLog.LevelError, "Error getting account types on repo", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	if accountTypes == nil {
		return []*mmodel.AccountType{}, cur, nil
	}

	ids := make([]string, len(accountTypes))
	for i := range accountTypes {
		ids[i] = accountTypes[i].ID.String()
	}

	metadataMap, err := uc.findPageMetadata(ctx, span, logger, constant.EntityAccountType, ids)
	if err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	for i := range accountTypes {
		if data, ok := metadataMap[accountTypes[i].ID.String()]; ok {
			accountTypes[i].Metadata = data
		}
	}

	return accountTypes, cur, nil
}
