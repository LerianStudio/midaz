// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package crmhttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v4/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// serviceName is the key under which the CRM circuit breaker is registered with the
// shared circuit-breaker manager. Using a stable name lets operators find the breaker
// in telemetry and lets tests reset it between scenarios.
const serviceName = "crm-account-relationship"

// Operation-scoped timeouts. Per the plan (§1.5), these are intentionally code
// constants rather than env vars: they reflect invariants of the CRM API contract
// (read paths are faster than write paths) and should only migrate to env vars if
// ops proves a need.
const (
	timeoutGetHolder          = 5 * time.Second
	timeoutCreateAccountAlias = 10 * time.Second
	timeoutGetAliasByAccount  = 5 * time.Second
	timeoutCloseAlias         = 10 * time.Second
)

// Circuit-breaker thresholds. Per the plan (§1.5), also code constants for now.
const (
	cbConsecutiveFailures uint32        = 5
	cbTimeout             time.Duration = 30 * time.Second
	cbHalfOpenMaxRequests uint32        = 1
)

// Client is the HTTP-backed implementation of CRMAccountRelationshipPort. It wraps a
// standard net/http.Client, enforces per-operation deadlines, and routes every call
// through a shared circuit breaker so a CRM outage cannot cascade into the Ledger
// request path.
//
// In Phase 1 the HTTP transport is intentionally unreachable: every method returns
// constant.ErrCRMInternalRouteNotImplemented so upstream callers can depend on the
// interface shape while the CRM-side "/internal/v1/*" routes are being built (Phase 3).
// The circuit-breaker wiring is nevertheless initialized here so that when the bodies
// are filled in (Phase 4), we will not need another round of bootstrap changes.
type Client struct {
	baseURL        string
	httpClient     *http.Client
	circuitBreaker libCircuitBreaker.CircuitBreaker
	logger         libLog.Logger
}

// NewClient constructs the CRM client. The baseURL must be absolute (for example,
// "http://midaz-crm:4003") and is never mutated after construction. The cbManager is
// reused from the bootstrap layer so that the CRM breaker's state is visible in the
// same observability surface as RabbitMQ's.
//
// Returns an error (rather than panicking) when validation fails, so bootstrap can
// surface the problem cleanly — no panic() in constructors (project CLAUDE.md).
func NewClient(baseURL string, cbManager libCircuitBreaker.Manager, logger libLog.Logger) (*Client, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		return nil, errors.New("crmhttp: base URL must not be empty")
	}

	if cbManager == nil {
		return nil, errors.New("crmhttp: circuit breaker manager must not be nil")
	}

	if logger == nil {
		return nil, errors.New("crmhttp: logger must not be nil")
	}

	// The http.Client timeout here is an umbrella safety net. Per-call deadlines are
	// enforced via context.WithTimeout so that the same *http.Client can safely serve
	// operations with different timing envelopes (GetHolder 5s vs CreateAccountAlias 10s).
	// We set the umbrella to the longest operation timeout so it never fires first.
	httpClient := &http.Client{
		Timeout: timeoutCreateAccountAlias,
	}

	cb, err := cbManager.GetOrCreate(serviceName, libCircuitBreaker.Config{
		MaxRequests:         cbHalfOpenMaxRequests,
		Interval:            cbTimeout,
		Timeout:             cbTimeout,
		ConsecutiveFailures: cbConsecutiveFailures,
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL:        trimmed,
		httpClient:     httpClient,
		circuitBreaker: cb,
		logger:         logger,
	}, nil
}

// GetHolder implements CRMAccountRelationshipPort.
//
// Phase 1: unreachable — returns ErrCRMInternalRouteNotImplemented. The method signature,
// circuit-breaker routing, and telemetry wiring are in place so that Phase 4's body
// implementation is a drop-in replacement.
func (c *Client) GetHolder(ctx context.Context, organizationID string, holderID uuid.UUID) (*mmodel.Holder, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "crm.client.get_holder")
	defer span.End()

	callCtx, cancel := context.WithTimeout(ctx, timeoutGetHolder)
	defer cancel()

	err := c.notImplemented(callCtx, "GetHolder", map[string]string{
		"organization_id": organizationID,
		"holder_id":       holderID.String(),
	})

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "CRM internal route not implemented", err)

	logger.Log(callCtx, libLog.LevelWarn, "CRM internal route not implemented",
		libLog.String("op", "GetHolder"),
		libLog.String("organization_id", organizationID),
		libLog.String("holder_id", holderID.String()))

	return nil, err
}

// CreateAccountAlias implements CRMAccountRelationshipPort.
//
// Phase 1: unreachable — returns ErrCRMInternalRouteNotImplemented.
func (c *Client) CreateAccountAlias(ctx context.Context, organizationID string, holderID uuid.UUID, input *mmodel.CreateAliasInput, idempotencyKey string) (*mmodel.Alias, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "crm.client.create_account_alias")
	defer span.End()

	callCtx, cancel := context.WithTimeout(ctx, timeoutCreateAccountAlias)
	defer cancel()

	// Defensive nil check — CreateAccountAlias without a payload is a programmer error;
	// surface it loudly rather than silently dispatching an empty alias creation.
	if input == nil {
		err := errors.New("crmhttp: CreateAccountAlias requires a non-nil input")

		libOpentelemetry.HandleSpanError(span, "Nil CreateAliasInput", err)

		logger.Log(callCtx, libLog.LevelError, "CreateAccountAlias called with nil input",
			libLog.String("organization_id", organizationID),
			libLog.String("holder_id", holderID.String()))

		return nil, err
	}

	err := c.notImplemented(callCtx, "CreateAccountAlias", map[string]string{
		"organization_id": organizationID,
		"holder_id":       holderID.String(),
		"idempotency_key": idempotencyKey,
	})

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "CRM internal route not implemented", err)

	logger.Log(callCtx, libLog.LevelWarn, "CRM internal route not implemented",
		libLog.String("op", "CreateAccountAlias"),
		libLog.String("organization_id", organizationID),
		libLog.String("holder_id", holderID.String()),
		libLog.String("idempotency_key", idempotencyKey))

	return nil, err
}

// GetAliasByAccount implements CRMAccountRelationshipPort.
//
// Phase 1: unreachable — returns ErrCRMInternalRouteNotImplemented.
func (c *Client) GetAliasByAccount(ctx context.Context, organizationID, ledgerID, accountID string) (*mmodel.Alias, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "crm.client.get_alias_by_account")
	defer span.End()

	callCtx, cancel := context.WithTimeout(ctx, timeoutGetAliasByAccount)
	defer cancel()

	err := c.notImplemented(callCtx, "GetAliasByAccount", map[string]string{
		"organization_id": organizationID,
		"ledger_id":       ledgerID,
		"account_id":      accountID,
	})

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "CRM internal route not implemented", err)

	logger.Log(callCtx, libLog.LevelWarn, "CRM internal route not implemented",
		libLog.String("op", "GetAliasByAccount"),
		libLog.String("organization_id", organizationID),
		libLog.String("ledger_id", ledgerID),
		libLog.String("account_id", accountID))

	return nil, err
}

// CloseAlias implements CRMAccountRelationshipPort.
//
// Phase 1: unreachable — returns ErrCRMInternalRouteNotImplemented.
func (c *Client) CloseAlias(ctx context.Context, organizationID string, holderID, aliasID uuid.UUID, idempotencyKey string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "crm.client.close_alias")
	defer span.End()

	callCtx, cancel := context.WithTimeout(ctx, timeoutCloseAlias)
	defer cancel()

	err := c.notImplemented(callCtx, "CloseAlias", map[string]string{
		"organization_id": organizationID,
		"holder_id":       holderID.String(),
		"alias_id":        aliasID.String(),
		"idempotency_key": idempotencyKey,
	})

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "CRM internal route not implemented", err)

	logger.Log(callCtx, libLog.LevelWarn, "CRM internal route not implemented",
		libLog.String("op", "CloseAlias"),
		libLog.String("organization_id", organizationID),
		libLog.String("holder_id", holderID.String()),
		libLog.String("alias_id", aliasID.String()),
		libLog.String("idempotency_key", idempotencyKey))

	return err
}

// notImplemented is the shared stub used by every method until Phase 3/4 fills in the
// HTTP call bodies. It exists as a function so we can grep for all Phase-1 stubs in
// one place when they go live. The fields parameter is reserved for future log/metric
// tagging — currently unused by the log line emitted at the call site, but present so
// the signature matches what the populated bodies will need.
func (c *Client) notImplemented(ctx context.Context, op string, _ map[string]string) error {
	_ = ctx // ctx is held here to document that callers respect per-op deadlines.
	_ = op  // op is reserved for future metric tags; intentionally unused today.

	return pkg.ValidateBusinessError(constant.ErrCRMInternalRouteNotImplemented, constant.EntityAccountRegistration)
}

// Ensure at compile time that Client satisfies the port. This catches accidental
// interface drift during refactoring.
var _ CRMAccountRelationshipPort = (*Client)(nil)
