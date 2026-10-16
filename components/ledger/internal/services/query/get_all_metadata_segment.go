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

// GetAllMetadataSegments fetches the page of segments of the ledger whose metadata matches the filter.
// No match yields an empty, non-nil page.
// The page is ordered by entity id in the filter's sort order.
func (uc *UseCase) GetAllMetadataSegments(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Segment, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_segments")
	defer span.End()

	return listMetadataWindow(ctx, span, logger, uc, filter, metadataListWindow[*mmodel.Segment]{
		collection: constant.EntitySegment,
		findAll: func(ctx context.Context, filter http.QueryHeader) ([]*mmodel.Segment, error) {
			return uc.SegmentRepo.FindAll(ctx, organizationID, ledgerID, filter)
		},
		entityID: func(s *mmodel.Segment) string { return s.ID },
		attach:   func(s *mmodel.Segment, data map[string]any) { s.Metadata = data },
	})
}
