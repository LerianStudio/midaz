// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// GetAllAccountType fetches all account types from the repository.
func (uc *UseCase) GetAllAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.AccountType, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_account_type")
	defer span.End()

	accountTypes, cur, err := uc.AccountTypeRepo.FindAll(ctx, organizationID, ledgerID, filter)
	if err != nil {
		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoAccountTypesFound, constant.EntityAccountType)

			logger.Log(ctx, libLog.LevelWarn, "No account types found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get account types on repo", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get account types on repo", err)

		logger.Log(ctx, libLog.LevelError, "Error getting account types on repo", libLog.Err(err))

		return nil, libHTTP.CursorPagination{}, err
	}

	if accountTypes != nil {
		metadataFilter := filter
		if metadataFilter.Metadata == nil {
			metadataFilter.Metadata = &bson.M{}
		}

		metadata, err := uc.OnboardingMetadataRepo.FindList(ctx, constant.EntityAccountType, metadataFilter)
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityAccountType)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb account type", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range accountTypes {
			if data, ok := metadataMap[accountTypes[i].ID.String()]; ok {
				accountTypes[i].Metadata = data
			}
		}
	}

	return accountTypes, cur, nil
}
