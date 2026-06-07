// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package midaz

import (
	"context"
	"errors"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	pkg "github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// activeStatusCode is the account status code that indicates an active account.
// Comparison is case-insensitive to handle both "active" and "ACTIVE".
const activeStatusCode = "active"

// errEmptyAccountTarget is a sentinel used only for span/log messages.
// The actual error returned to callers is the mapped constant.ErrInvalidAccountTarget.
var errEmptyAccountTarget = "account target has no resolution criteria (segmentId, portfolioId, or aliases)"

// AccountResolver resolves accounts by criteria defined in an AccountTarget.
type AccountResolver interface {
	ResolveAccounts(ctx context.Context, orgID, ledgerID uuid.UUID, target model.AccountTarget) ([]feeshared.Account, error)
}

// ErrNilResolver is returned when a nil MidazResolver is provided to NewAccountResolver.
var ErrNilResolver = errors.New("MidazResolver is required and cannot be nil")

// midazAccountResolver implements AccountResolver by delegating to the in-process MidazResolver.
type midazAccountResolver struct {
	resolver feeshared.MidazResolver
}

// NewAccountResolver creates a new AccountResolver backed by the given MidazResolver.
func NewAccountResolver(resolver feeshared.MidazResolver) (AccountResolver, error) {
	if resolver == nil {
		return nil, ErrNilResolver
	}

	return &midazAccountResolver{resolver: resolver}, nil
}

// ResolveAccounts resolves accounts matching the given target criteria,
// filtering out inactive accounts (status != "active").
func (r *midazAccountResolver) ResolveAccounts(
	ctx context.Context,
	orgID, ledgerID uuid.UUID,
	target model.AccountTarget,
) ([]feeshared.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "billing.resolve_accounts")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", orgID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	var accounts []feeshared.Account

	var err error

	switch {
	case target.SegmentID != nil:
		span.SetAttributes(attribute.String("app.request.target_type", "segmentId"))

		accounts, err = r.resolver.ListAccounts(ctx, orgID, ledgerID, target.SegmentID, nil)

	case target.PortfolioID != nil:
		span.SetAttributes(attribute.String("app.request.target_type", "portfolioId"))

		accounts, err = r.resolver.ListAccounts(ctx, orgID, ledgerID, nil, target.PortfolioID)

	case len(target.Aliases) > 0:
		span.SetAttributes(attribute.String("app.request.target_type", "aliases"))

		accounts, err = r.resolveByAliases(ctx, orgID, ledgerID, target.Aliases)

	default:
		bizErr := pkg.ValidateBusinessError(constant.ErrInvalidAccountTarget, "",
			errEmptyAccountTarget)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty account target", bizErr)
		logger.Log(ctx, libLog.LevelWarn, "Empty account target")

		return nil, bizErr
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve accounts", err)
		logger.Log(ctx, libLog.LevelError, "Error resolving accounts", libLog.Err(err))

		return nil, err
	}

	active := filterActiveAccounts(accounts)

	span.SetAttributes(
		attribute.Int("app.response.total_accounts", len(accounts)),
		attribute.Int("app.response.active_accounts", len(active)),
	)

	return active, nil
}

// resolveByAliases resolves each alias individually via GetAccountByAlias.
func (r *midazAccountResolver) resolveByAliases(
	ctx context.Context,
	orgID, ledgerID uuid.UUID,
	aliases []string,
) ([]feeshared.Account, error) {
	// Deduplicate aliases to avoid resolving the same account multiple times,
	// which would cause downstream billing to double-charge.
	seen := make(map[string]struct{}, len(aliases))
	unique := make([]string, 0, len(aliases))

	for _, a := range aliases {
		if _, exists := seen[a]; !exists {
			seen[a] = struct{}{}
			unique = append(unique, a)
		}
	}

	accounts := make([]feeshared.Account, 0, len(unique))

	for _, alias := range unique {
		account, err := r.resolver.GetAccountByAlias(ctx, orgID, ledgerID, alias)
		if err != nil {
			return nil, err
		}

		if account == nil {
			continue
		}

		accounts = append(accounts, *account)
	}

	return accounts, nil
}

// filterActiveAccounts returns only accounts whose status code is "active" (case-insensitive).
func filterActiveAccounts(accounts []feeshared.Account) []feeshared.Account {
	active := make([]feeshared.Account, 0, len(accounts))

	for _, acc := range accounts {
		if acc.Status != nil && strings.EqualFold(acc.Status.Code, activeStatusCode) {
			active = append(active, acc)
		}
	}

	return active
}
