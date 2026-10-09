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

// GetAllMetadataLedgers fetches the page of ledgers of the organization whose metadata matches the
// filter. No match yields an empty, non-nil page.
// The page is ordered by entity id in the filter's sort order.
func (uc *UseCase) GetAllMetadataLedgers(ctx context.Context, organizationID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Ledger, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_ledgers")
	defer span.End()

	return listMetadataWindow(ctx, span, logger, uc, filter, metadataListWindow[*mmodel.Ledger]{
		collection: constant.EntityLedger,
		findAll: func(ctx context.Context, filter http.QueryHeader) ([]*mmodel.Ledger, error) {
			return uc.LedgerRepo.FindAll(ctx, organizationID, filter)
		},
		entityID: func(l *mmodel.Ledger) string { return l.ID },
		attach:   func(l *mmodel.Ledger, data map[string]any) { l.Metadata = data },
	})
}
