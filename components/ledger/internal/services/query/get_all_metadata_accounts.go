// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// GetAllMetadataAccounts fetches the page of accounts of the ledger whose metadata matches the
// filter, narrowed to the portfolio and segment when given. No match yields an empty, non-nil page.
// The page is ordered by entity id in the filter's sort order.
func (uc *UseCase) GetAllMetadataAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID, segmentID *uuid.UUID, filter http.QueryHeader, holderPolicy mmodel.HolderPolicy) ([]*mmodel.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_accounts")
	defer span.End()

	return listMetadataWindow(ctx, span, logger, uc, filter, metadataListWindow[*mmodel.Account]{
		collection: constant.EntityAccount,
		findAll: func(ctx context.Context, filter http.QueryHeader) ([]*mmodel.Account, error) {
			return uc.AccountRepo.FindAll(ctx, organizationID, ledgerID, portfolioID, segmentID, filter, holderPolicy)
		},
		entityID: func(a *mmodel.Account) string { return a.ID },
		attach:   func(a *mmodel.Account, data map[string]any) { a.Metadata = data },
	})
}
