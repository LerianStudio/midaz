// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"slices"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// billingAliasDetach is the write that removes one alias from one billing package.
// disables is true only when the write turns an enabled package off.
type billingAliasDetach struct {
	updateFields bson.M
	actions      []string
	disables     bool
}

// DetachAccountAlias removes every reference to alias from the ledger's
// non-deleted billing packages, enabled or not. A posting leg naming alias
// (debitAccountAlias, creditAccountAlias, maintenanceCreditAccount) is unset and
// disables the package; alias leaves accountTarget.aliases, disabling the
// package only when no target alias is left. Packages that do not reference
// alias are not written, and a package deleted meanwhile is skipped. It reports
// how many packages it updated and how many of them it disabled.
func (s *BillingPackageService) DetachAccountAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (updated, disabled int, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.detach_billing_account_alias")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, s.MetricsFactory, logger, "fees", "detach_billing_account_alias", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.alias", alias),
	)

	packages, err := s.billingPackageRepo.FindNotDeletedByLedger(ctx, organizationID.String(), ledgerID.String())
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list billing packages to detach account alias", err)

		return 0, 0, err
	}

	for _, bp := range packages {
		detach, matched := buildBillingAliasDetach(bp, alias)
		if !matched {
			continue
		}

		result, errUpdate := s.billingPackageRepo.Update(ctx, bp.ID, organizationID.String(), ledgerID.String(), &detach.updateFields)
		if errUpdate != nil {
			var notFound pkg.EntityNotFoundError
			if errors.As(errUpdate, &notFound) {
				continue
			}

			libOpentelemetry.HandleSpanError(span, "Failed to detach account alias from billing package", errUpdate)

			return 0, 0, errUpdate
		}

		s.emitBillingPackageUpdatedEvent(ctx, span, logger, result)

		for _, action := range detach.actions {
			logger.Log(ctx, libLog.LevelInfo, "Detached deleted account alias from billing package",
				libLog.String("package_id", bp.ID),
				libLog.String("action", action),
			)
		}

		updated++

		if detach.disables {
			disabled++
		}
	}

	span.SetAttributes(
		attribute.Int("app.billing_packages_updated", updated),
		attribute.Int("app.billing_packages_disabled", disabled),
	)

	return updated, disabled, nil
}

// buildBillingAliasDetach builds the write that removes alias from bp. It
// reports false when bp does not reference alias.
func buildBillingAliasDetach(bp *model.BillingPackage, alias string) (billingAliasDetach, bool) {
	legs := []struct {
		field string
		value *string
	}{
		{"debit_account_alias", bp.DebitAccountAlias},
		{"credit_account_alias", bp.CreditAccountAlias},
		{"maintenance_credit_account", bp.MaintenanceCreditAccount},
	}

	unsetFields := bson.M{}

	for _, leg := range legs {
		if leg.value != nil && *leg.value == alias {
			unsetFields[leg.field] = ""
		}
	}

	targeted := bp.AccountTarget != nil && slices.Contains(bp.AccountTarget.Aliases, alias)

	if len(unsetFields) == 0 && !targeted {
		return billingAliasDetach{}, false
	}

	detach := billingAliasDetach{updateFields: bson.M{}}
	setFields := bson.M{}
	disable := len(unsetFields) > 0

	if len(unsetFields) > 0 {
		detach.updateFields["$unset"] = unsetFields
		detach.actions = append(detach.actions, "leg_unset")
	}

	if targeted {
		detach.updateFields["$pull"] = bson.M{"account_target.aliases": alias}
		detach.actions = append(detach.actions, "alias_pulled")

		remaining := slices.DeleteFunc(slices.Clone(bp.AccountTarget.Aliases), func(a string) bool { return a == alias })
		if len(remaining) == 0 {
			disable = true
		}
	}

	if disable {
		setFields["enable"] = false

		if bp.Enable != nil && *bp.Enable {
			detach.disables = true
			detach.actions = append(detach.actions, "disabled")
		}
	}

	setFields["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	detach.updateFields["$set"] = setFields

	return detach, true
}
