// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpenTelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"go.opentelemetry.io/otel/attribute"
)

// GetAliasByAccount returns the single non-deleted alias for the given
// (ledger_id, account_id) pair. Uniqueness is guaranteed at the persistence
// layer by a unique index. Callers that receive ErrAliasNotFound should treat
// it as "no CRM record exists for this Ledger account" — distinct from a
// tenant-resolution failure (which propagates from the middleware as 401).
func (uc *UseCase) GetAliasByAccount(ctx context.Context, organizationID, ledgerID, accountID string) (*mmodel.Alias, error) {
	logger, tracer, reqID, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.get_alias_by_account")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.ledger_id", ledgerID),
		attribute.String("app.request.account_id", accountID),
	)

	alias, err := uc.AliasRepo.FindByLedgerAndAccount(ctx, organizationID, ledgerID, accountID)
	if err != nil {
		libOpenTelemetry.HandleSpanError(span, "Failed to find alias by ledger+account", err)
		logger.Log(ctx, libLog.LevelWarn, "Alias lookup by ledger+account failed", libLog.Err(err))

		return nil, err
	}

	return alias, nil
}
