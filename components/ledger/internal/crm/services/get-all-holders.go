// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// GetAllHolders retrieves holders that match the query filter.
func (uc *UseCase) GetAllHolders(ctx context.Context, organizationID string, filter http.QueryHeader, includeDeleted bool) (_ []*mmodel.Holder, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_all_holders")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "crm", "list_holders", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID),
	)

	filter, listsNothing, err := uc.confineHolders(ctx, organizationID, filter)
	if err != nil {
		recordSpanError(span, "Failed to confine holders to the scope", err)

		return nil, err
	}

	if listsNothing {
		return []*mmodel.Holder{}, nil
	}

	holders, err := uc.HolderRepo.FindAll(ctx, organizationID, filter, includeDeleted)
	if err != nil {
		recordSpanError(span, "Failed to get holders", err)

		return nil, err
	}

	return holders, nil
}

// HolderScopeReader reads the holders that own a live account of the
// organization within a scope confined on ledgerId and accountId.
type HolderScopeReader interface {
	HolderIDsInScope(ctx context.Context, organizationID uuid.UUID, scope http.ScopeConfinement) ([]uuid.UUID, error)
}

// errHolderScopeUnavailable refuses a scoped listing the use case has no way to
// confine.
var errHolderScopeUnavailable = errors.New("holder listing is confined but no holder scope reader is configured")

// confineHolders translates the ledger and account confinement of filter into
// the holders that own an account within it. listsNothing reports a
// confinement no holder satisfies.
func (uc *UseCase) confineHolders(ctx context.Context, organizationID string, filter http.QueryHeader) (http.QueryHeader, bool, error) {
	if len(filter.Scope) == 0 {
		return filter, false, nil
	}

	if filter.Scope.ListsNothing() {
		return filter, true, nil
	}

	if uc.HolderScope == nil {
		return filter, false, errHolderScopeUnavailable
	}

	orgID, err := uuid.Parse(organizationID)
	if err != nil {
		return filter, false, fmt.Errorf("organization id %q is not a uuid: %w", organizationID, err)
	}

	ids, err := uc.HolderScope.HolderIDsInScope(ctx, orgID, filter.Scope)
	if err != nil {
		return filter, false, err
	}

	if len(ids) == 0 {
		return filter, true, nil
	}

	filter.Scope = http.ScopeConfinement{"holderId": ids}

	return filter, false, nil
}
