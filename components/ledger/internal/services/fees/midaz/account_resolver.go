// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package midaz

import (
	"context"
	"fmt"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// accountPageSize is the number of accounts fetched per page when paginating ListAccounts.
const accountPageSize = 100

// activeStatusCode is the account status code that indicates an active account.
// Comparison is case-insensitive to handle both "active" and "ACTIVE" from Midaz.
const activeStatusCode = "active"

// errEmptyAccountTarget is a sentinel used only for span/log messages.
// The actual error returned to callers is the mapped constant.ErrInvalidAccountTarget.
var errEmptyAccountTarget = "account target has no resolution criteria (segmentId, portfolioId, or aliases)"

// AccountResolver resolves accounts by criteria defined in an AccountTarget.
type AccountResolver interface {
	ResolveAccounts(ctx context.Context, orgID, ledgerID uuid.UUID, target model.AccountTarget) ([]pkg.Account, error)
}

// midazAccountResolver implements AccountResolver by delegating to MidazClient.
type midazAccountResolver struct {
	client http.MidazClient
}

// NewAccountResolver creates a new AccountResolver backed by the given MidazClient.
func NewAccountResolver(client http.MidazClient) (AccountResolver, error) {
	if client == nil {
		return nil, ErrNilMidazClient
	}

	return &midazAccountResolver{client: client}, nil
}

// ResolveAccounts resolves accounts matching the given target criteria,
// filtering out inactive accounts (status != "active").
func (r *midazAccountResolver) ResolveAccounts(
	ctx context.Context,
	orgID, ledgerID uuid.UUID,
	target model.AccountTarget,
) ([]pkg.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "billing.resolve_accounts")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", orgID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	var accounts []pkg.Account

	var err error

	switch {
	case target.SegmentID != nil:
		span.SetAttributes(attribute.String("app.request.target_type", "segmentId"))

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Resolving accounts by segmentId: org=%s, ledger=%s, segment=%s",
			orgID.String(), ledgerID.String(), target.SegmentID.String()))

		accounts, err = r.resolveByListAccounts(ctx, orgID, ledgerID, http.AccountFilters{
			SegmentID: target.SegmentID,
		})

	case target.PortfolioID != nil:
		span.SetAttributes(attribute.String("app.request.target_type", "portfolioId"))

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Resolving accounts by portfolioId: org=%s, ledger=%s, portfolio=%s",
			orgID.String(), ledgerID.String(), target.PortfolioID.String()))

		accounts, err = r.resolveByListAccounts(ctx, orgID, ledgerID, http.AccountFilters{
			PortfolioID: target.PortfolioID,
		})

	case len(target.Aliases) > 0:
		span.SetAttributes(attribute.String("app.request.target_type", "aliases"))

		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Resolving accounts by aliases: org=%s, ledger=%s, count=%d",
			orgID.String(), ledgerID.String(), len(target.Aliases)))

		accounts, err = r.resolveByAliases(ctx, orgID, ledgerID, target.Aliases)

	default:
		bizErr := pkg.ValidateBusinessError(constant.ErrInvalidAccountTarget, "",
			errEmptyAccountTarget)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty account target", bizErr)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Empty account target: org=%s, ledger=%s", orgID.String(), ledgerID.String()))

		return nil, bizErr
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve accounts", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error resolving accounts: %v", err))

		return nil, err
	}

	active := filterActiveAccounts(accounts)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Resolved accounts: total=%d, active=%d", len(accounts), len(active)))

	span.SetAttributes(
		attribute.Int("app.response.total_accounts", len(accounts)),
		attribute.Int("app.response.active_accounts", len(active)),
	)

	return active, nil
}

// resolveByListAccounts paginates through ListAccounts until all pages are consumed.
func (r *midazAccountResolver) resolveByListAccounts(
	ctx context.Context,
	orgID, ledgerID uuid.UUID,
	filters http.AccountFilters,
) ([]pkg.Account, error) {
	var allAccounts []pkg.Account

	page := 1

	for {
		accountPage, err := r.client.ListAccounts(ctx, orgID, ledgerID, filters, page, accountPageSize)
		if err != nil {
			return nil, err
		}

		if accountPage == nil {
			return nil, pkg.ValidateBusinessError(constant.ErrMidazQueryFailed, "", "ListAccounts returned nil page without error")
		}

		allAccounts = append(allAccounts, accountPage.Items...)

		if len(accountPage.Items) < accountPageSize {
			break
		}

		page++
	}

	return allAccounts, nil
}

// resolveByAliases resolves each alias individually via GetAccountDetailsByAlias.
func (r *midazAccountResolver) resolveByAliases(
	ctx context.Context,
	orgID, ledgerID uuid.UUID,
	aliases []string,
) ([]pkg.Account, error) {
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

	accounts := make([]pkg.Account, 0, len(unique))

	orgIDStr := orgID.String()
	ledgerIDStr := ledgerID.String()

	for _, alias := range unique {
		account, err := r.client.GetAccountDetailsByAlias(ctx, orgIDStr, ledgerIDStr, alias)
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
func filterActiveAccounts(accounts []pkg.Account) []pkg.Account {
	active := make([]pkg.Account, 0, len(accounts))

	for _, acc := range accounts {
		if acc.Status != nil && strings.EqualFold(acc.Status.Code, activeStatusCode) {
			active = append(active, acc)
		}
	}

	return active
}
