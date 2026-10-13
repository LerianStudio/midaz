// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// GetAllMetadataOrganizations fetches the page of organizations whose metadata matches the filter.
// No match yields an empty, non-nil page.
// The page is ordered by entity id in the filter's sort order.
func (uc *UseCase) GetAllMetadataOrganizations(ctx context.Context, filter http.QueryHeader) ([]*mmodel.Organization, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_organizations")
	defer span.End()

	return listMetadataWindow(ctx, span, logger, uc, filter, metadataListWindow[*mmodel.Organization]{
		collection: constant.EntityOrganization,
		findAll: func(ctx context.Context, filter http.QueryHeader) ([]*mmodel.Organization, error) {
			return uc.OrganizationRepo.FindAll(ctx, filter)
		},
		entityID: func(o *mmodel.Organization) string { return o.ID },
		attach:   func(o *mmodel.Organization, data map[string]any) { o.Metadata = data },
	})
}
