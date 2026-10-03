// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// HolderIDsInScope returns the holders that own a live account of the
// organization within scope, confined on ledgerId and accountId.
func (uc *UseCase) HolderIDsInScope(ctx context.Context, organizationID uuid.UUID, scope http.ScopeConfinement) ([]uuid.UUID, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.holder_ids_in_scope")
	defer span.End()

	ids, err := uc.AccountRepo.ListHolderIDs(ctx, organizationID, scope)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list the holders in scope", err)

		return nil, err
	}

	return ids, nil
}
