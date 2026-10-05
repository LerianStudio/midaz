// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// detachPackageMaxAttempts bounds the optimistic writes tried on one fee package
// before the detach gives up on a package that keeps changing underneath it.
const detachPackageMaxAttempts = 3

// errDetachConflictExhausted reports a fee package whose updatedAt kept moving
// across every detach attempt.
var errDetachConflictExhausted = errors.New("fee package kept changing while detaching account alias")

// errNilBillingPackages reports a UseCase wired without its billing package service.
var errNilBillingPackages = errors.New("billing package service is required to detach an account alias")

// packageAliasDetach is the write that removes one alias from one fee package.
// disables is true only when the write turns an enabled package off.
type packageAliasDetach struct {
	updateFields bson.M
	actions      []string
	disables     bool
}

// DetachAccountAlias removes every reference to alias from the ledger's
// non-deleted fee packages and then from its billing packages, enabled or not.
// In a fee package a fee crediting alias is removed whole, alias leaves
// waivedAccounts, and a package left without fees is disabled; billing packages
// follow BillingPackageService.DetachAccountAlias. Packages that do not
// reference alias are not written. The first failure aborts and is returned;
// packages already written stay written, and a repeated call finds nothing left
// to change in them.
func (uc *UseCase) DetachAccountAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (result model.FeeAliasDetachResult, err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.detach_account_alias")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "fees", "detach_account_alias", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.alias", alias),
	)

	if uc.BillingPackages == nil {
		err = errNilBillingPackages
		libOpentelemetry.HandleSpanError(span, "Fee use case wired without billing package service", err)

		return model.FeeAliasDetachResult{}, err
	}

	result, err = uc.detachAliasFromPacks(ctx, span, logger, organizationID, ledgerID, alias)
	if err != nil {
		return model.FeeAliasDetachResult{}, err
	}

	billingUpdated, billingDisabled, err := uc.BillingPackages.DetachAccountAlias(ctx, organizationID, ledgerID, alias)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to detach account alias from billing packages", err)

		return model.FeeAliasDetachResult{}, err
	}

	result.PackagesUpdated += billingUpdated
	result.PackagesDisabled += billingDisabled

	span.SetAttributes(
		attribute.Int("app.fee_packages_updated", result.PackagesUpdated),
		attribute.Int("app.fee_packages_disabled", result.PackagesDisabled),
	)

	return result, nil
}

// detachAliasFromPacks applies the fee-package half of DetachAccountAlias. The
// package cache is invalidated once whenever at least one package was written,
// including when a later package fails.
func (uc *UseCase) detachAliasFromPacks(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, alias string) (model.FeeAliasDetachResult, error) {
	var result model.FeeAliasDetachResult

	packages, err := uc.packageRepo.FindNotDeletedByOrganizationIDAndLedgerID(ctx, organizationID, ledgerID)
	if err != nil {
		spanattr.HandleSpanByErrorClass(span, "Failed to list fee packages to detach account alias", err)

		return result, err
	}

	defer func() {
		if result.PackagesUpdated > 0 {
			uc.invalidatePackageCache(ctx, logger, organizationID, ledgerID)
		}
	}()

	for _, p := range packages {
		changed, disabled, errPack := uc.detachAliasFromPack(ctx, span, logger, organizationID, ledgerID, p, alias)
		if errPack != nil {
			spanattr.HandleSpanByErrorClass(span, "Failed to detach account alias from fee package", errPack)

			return result, errPack
		}

		if changed {
			result.PackagesUpdated++
		}

		if disabled {
			result.PackagesDisabled++
		}
	}

	return result, nil
}

// detachAliasFromPack writes one package under its updatedAt. A write that
// matches nothing means the package changed or was deleted since it was read:
// it is read again and, while it still references alias, written again.
func (uc *UseCase) detachAliasFromPack(ctx context.Context, span trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, p *pack.Package, alias string) (changed, disabled bool, err error) {
	current := p

	for attempt := 1; ; attempt++ {
		detach, matched := buildPackageAliasDetach(current, alias)
		if !matched {
			return false, false, nil
		}

		updated, errUpdate := uc.packageRepo.Update(ctx, current.ID, organizationID, ledgerID, current.UpdatedAt, &detach.updateFields)
		if errUpdate == nil {
			uc.emitFeesPackageUpdatedEvent(ctx, span, logger, updated, organizationID)

			for _, action := range detach.actions {
				logger.Log(ctx, libLog.LevelInfo, "Detached deleted account alias from fee package",
					libLog.String("package_id", current.ID.String()),
					libLog.String("action", action),
				)
			}

			return true, detach.disables, nil
		}

		var notFound pkg.EntityNotFoundError
		if !errors.As(errUpdate, &notFound) {
			return false, false, errUpdate
		}

		if attempt >= detachPackageMaxAttempts {
			return false, false, fmt.Errorf("%w: package %s", errDetachConflictExhausted, current.ID)
		}

		fresh, errFind := uc.packageRepo.FindByID(ctx, current.ID, organizationID, ledgerID)
		if errors.Is(errFind, mongo.ErrNoDocuments) {
			return false, false, nil
		}

		if errFind != nil {
			return false, false, errFind
		}

		current = fresh
	}
}

// buildPackageAliasDetach builds the write that removes alias from p, mirroring
// the package PATCH: each fee crediting alias is unset, waivedAccounts is
// rewritten without alias, and a package left without fees is disabled. It
// reports false when p does not reference alias.
func buildPackageAliasDetach(p *pack.Package, alias string) (packageAliasDetach, bool) {
	unsetFields := bson.M{}

	for key, fee := range p.Fees {
		if fee.CreditAccount == alias {
			unsetFields["fees."+key] = ""
		}
	}

	waived := p.WaivedAccounts != nil && slices.Contains(*p.WaivedAccounts, alias)

	if len(unsetFields) == 0 && !waived {
		return packageAliasDetach{}, false
	}

	setFields := bson.M{}
	detach := packageAliasDetach{}

	if len(unsetFields) > 0 {
		detach.actions = append(detach.actions, "fee_removed")
	}

	if waived {
		setFields["waived_accounts"] = slices.DeleteFunc(slices.Clone(*p.WaivedAccounts), func(a string) bool { return a == alias })
		detach.actions = append(detach.actions, "alias_waiver_removed")
	}

	if len(p.Fees)-len(unsetFields) == 0 {
		setFields["enable"] = false

		if enableOrFalse(p.Enable) {
			detach.disables = true
			detach.actions = append(detach.actions, "disabled")
		}
	}

	setFields["updated_at"] = time.Now()

	detach.updateFields = bson.M{"$set": setFields}
	if len(unsetFields) > 0 {
		detach.updateFields["$unset"] = unsetFields
	}

	return detach, true
}
