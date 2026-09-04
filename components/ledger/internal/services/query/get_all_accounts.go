// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// GetAllAccount fetches all accounts from the repository.
func (uc *UseCase) GetAllAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID, segmentID *uuid.UUID, filter http.QueryHeader, holderPolicy mmodel.HolderPolicy) (_ []*mmodel.Account, err error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_account")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "ledger", "list_accounts", start, err)
	}()

	accounts, err := uc.AccountRepo.FindAll(ctx, organizationID, ledgerID, portfolioID, segmentID, filter, holderPolicy)
	if err != nil {
		logger.Log(ctx, libLog.LevelError, "Error getting accounts on repo", libLog.Err(err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoAccountsFound, constant.EntityAccount)

			logger.Log(ctx, libLog.LevelWarn, "No accounts found")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get accounts on repo", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get accounts on repo", err)

		return nil, err
	}

	return uc.attachAccountMetadata(ctx, span, accounts)
}

// attachAccountMetadata enriches a page of accounts with their onboarding
// metadata. A metadata lookup failure surfaces as ErrNoAccountsFound, the same
// class both account listings report.
func (uc *UseCase) attachAccountMetadata(ctx context.Context, span trace.Span, accounts []*mmodel.Account) ([]*mmodel.Account, error) {
	if len(accounts) == 0 {
		return accounts, nil
	}

	accountIDs := make([]string, len(accounts))
	for i, a := range accounts {
		accountIDs[i] = a.ID
	}

	metadata, err := uc.OnboardingMetadataRepo.FindByEntityIDs(ctx, constant.EntityAccount, accountIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoAccountsFound, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on repo", err)

		return nil, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	for i := range accounts {
		if data, ok := metadataMap[accounts[i].ID]; ok {
			accounts[i].Metadata = data
		}
	}

	return accounts, nil
}
