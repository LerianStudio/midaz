// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package crmhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
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
	"go.opentelemetry.io/otel/trace"
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
type Client struct {
	baseURL        string
	httpClient     *nethttp.Client
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
	httpClient := &nethttp.Client{
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
func (c *Client) GetHolder(ctx context.Context, organizationID string, holderID uuid.UUID, token string) (*mmodel.Holder, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "crm.client.get_holder")
	defer span.End()

	callCtx, cancel := context.WithTimeout(ctx, timeoutGetHolder)
	defer cancel()

	url := c.baseURL + "/v1/holders/" + holderID.String()

	body, status, err := c.doRequest(callCtx, nethttp.MethodGet, url, nil, token, "")
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "CRM GetHolder transport failure", err)
		logger.Log(callCtx, libLog.LevelWarn, "CRM GetHolder transport failure",
			libLog.String("organization_id", organizationID),
			libLog.String("holder_id", holderID.String()),
			libLog.Err(err))

		return nil, pkg.ValidateBusinessError(constant.ErrCRMTransient, constant.EntityAccountRegistration)
	}

	if status == nethttp.StatusOK {
		holder := &mmodel.Holder{}
		if decodeErr := json.Unmarshal(body, holder); decodeErr != nil {
			libOpentelemetry.HandleSpanError(span, "CRM GetHolder decode failure", decodeErr)
			logger.Log(callCtx, libLog.LevelError, "CRM GetHolder decode failure",
				libLog.String("holder_id", holderID.String()),
				libLog.Err(decodeErr))

			return nil, pkg.ValidateBusinessError(constant.ErrCRMTransient, constant.EntityAccountRegistration)
		}

		return holder, nil
	}

	if status == nethttp.StatusNotFound {
		businessErr := pkg.ValidateBusinessError(constant.ErrHolderNotFound, constant.EntityAccountRegistration)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "CRM holder not found", businessErr)
		logger.Log(callCtx, libLog.LevelWarn, "CRM holder not found",
			libLog.String("organization_id", organizationID),
			libLog.String("holder_id", holderID.String()))

		return nil, businessErr
	}

	return nil, c.classifyErrorStatus(span, logger, callCtx, "GetHolder", status, body, map[string]string{
		"organization_id": organizationID,
		"holder_id":       holderID.String(),
	})
}

// CreateAccountAlias implements CRMAccountRelationshipPort.
func (c *Client) CreateAccountAlias(ctx context.Context, organizationID string, holderID uuid.UUID, input *mmodel.CreateAliasInput, idempotencyKey, token string) (*mmodel.Alias, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "crm.client.create_account_alias")
	defer span.End()

	callCtx, cancel := context.WithTimeout(ctx, timeoutCreateAccountAlias)
	defer cancel()

	if input == nil {
		err := errors.New("crmhttp: CreateAccountAlias requires a non-nil input")

		libOpentelemetry.HandleSpanError(span, "Nil CreateAliasInput", err)
		logger.Log(callCtx, libLog.LevelError, "CreateAccountAlias called with nil input",
			libLog.String("organization_id", organizationID),
			libLog.String("holder_id", holderID.String()))

		return nil, err
	}

	payload, err := json.Marshal(input)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal CreateAliasInput", err)
		logger.Log(callCtx, libLog.LevelError, "Failed to marshal CreateAliasInput",
			libLog.String("holder_id", holderID.String()),
			libLog.Err(err))

		return nil, fmt.Errorf("crmhttp: marshal alias input: %w", err)
	}

	url := c.baseURL + "/v1/holders/" + holderID.String() + "/aliases"

	body, status, err := c.doRequest(callCtx, nethttp.MethodPost, url, payload, token, idempotencyKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "CRM CreateAccountAlias transport failure", err)
		logger.Log(callCtx, libLog.LevelWarn, "CRM CreateAccountAlias transport failure",
			libLog.String("organization_id", organizationID),
			libLog.String("holder_id", holderID.String()),
			libLog.String("idempotency_key", idempotencyKey),
			libLog.Err(err))

		return nil, pkg.ValidateBusinessError(constant.ErrCRMTransient, constant.EntityAccountRegistration)
	}

	if status == nethttp.StatusCreated || status == nethttp.StatusOK {
		alias := &mmodel.Alias{}
		if decodeErr := json.Unmarshal(body, alias); decodeErr != nil {
			libOpentelemetry.HandleSpanError(span, "CRM CreateAccountAlias decode failure", decodeErr)
			logger.Log(callCtx, libLog.LevelError, "CRM CreateAccountAlias decode failure",
				libLog.String("holder_id", holderID.String()),
				libLog.Err(decodeErr))

			return nil, pkg.ValidateBusinessError(constant.ErrCRMTransient, constant.EntityAccountRegistration)
		}

		return alias, nil
	}

	return nil, c.classifyErrorStatus(span, logger, callCtx, "CreateAccountAlias", status, body, map[string]string{
		"organization_id": organizationID,
		"holder_id":       holderID.String(),
		"idempotency_key": idempotencyKey,
	})
}

// GetAliasByAccount implements CRMAccountRelationshipPort.
func (c *Client) GetAliasByAccount(ctx context.Context, organizationID, ledgerID, accountID, token string) (*mmodel.Alias, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "crm.client.get_alias_by_account")
	defer span.End()

	callCtx, cancel := context.WithTimeout(ctx, timeoutGetAliasByAccount)
	defer cancel()

	url := fmt.Sprintf("%s/v1/aliases/by-account?ledger_id=%s&account_id=%s",
		c.baseURL, ledgerID, accountID)

	body, status, err := c.doRequest(callCtx, nethttp.MethodGet, url, nil, token, "")
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "CRM GetAliasByAccount transport failure", err)
		logger.Log(callCtx, libLog.LevelWarn, "CRM GetAliasByAccount transport failure",
			libLog.String("organization_id", organizationID),
			libLog.String("ledger_id", ledgerID),
			libLog.String("account_id", accountID),
			libLog.Err(err))

		return nil, pkg.ValidateBusinessError(constant.ErrCRMTransient, constant.EntityAccountRegistration)
	}

	if status == nethttp.StatusOK {
		alias := &mmodel.Alias{}
		if decodeErr := json.Unmarshal(body, alias); decodeErr != nil {
			libOpentelemetry.HandleSpanError(span, "CRM GetAliasByAccount decode failure", decodeErr)
			logger.Log(callCtx, libLog.LevelError, "CRM GetAliasByAccount decode failure",
				libLog.String("account_id", accountID),
				libLog.Err(decodeErr))

			return nil, pkg.ValidateBusinessError(constant.ErrCRMTransient, constant.EntityAccountRegistration)
		}

		return alias, nil
	}

	// 404 is saga-specific: "no prior attempt created an alias for this account".
	// Surface the raw sentinel (no ValidateBusinessError wrap) so the saga can
	// errors.Is-check it cleanly without triggering the HTTP-level 404 response.
	if status == nethttp.StatusNotFound {
		logger.Log(callCtx, libLog.LevelDebug, "CRM reports no alias for account",
			libLog.String("ledger_id", ledgerID),
			libLog.String("account_id", accountID))

		return nil, constant.ErrAliasNotFound
	}

	return nil, c.classifyErrorStatus(span, logger, callCtx, "GetAliasByAccount", status, body, map[string]string{
		"organization_id": organizationID,
		"ledger_id":       ledgerID,
		"account_id":      accountID,
	})
}

// CloseAlias implements CRMAccountRelationshipPort.
func (c *Client) CloseAlias(ctx context.Context, organizationID string, holderID, aliasID uuid.UUID, idempotencyKey, token string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "crm.client.close_alias")
	defer span.End()

	callCtx, cancel := context.WithTimeout(ctx, timeoutCloseAlias)
	defer cancel()

	url := fmt.Sprintf("%s/v1/holders/%s/aliases/%s/close",
		c.baseURL, holderID.String(), aliasID.String())

	body, status, err := c.doRequest(callCtx, nethttp.MethodPost, url, nil, token, idempotencyKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "CRM CloseAlias transport failure", err)
		logger.Log(callCtx, libLog.LevelWarn, "CRM CloseAlias transport failure",
			libLog.String("organization_id", organizationID),
			libLog.String("holder_id", holderID.String()),
			libLog.String("alias_id", aliasID.String()),
			libLog.Err(err))

		return pkg.ValidateBusinessError(constant.ErrCRMTransient, constant.EntityAccountRegistration)
	}

	if status == nethttp.StatusOK || status == nethttp.StatusNoContent {
		return nil
	}

	return c.classifyErrorStatus(span, logger, callCtx, "CloseAlias", status, body, map[string]string{
		"organization_id": organizationID,
		"holder_id":       holderID.String(),
		"alias_id":        aliasID.String(),
		"idempotency_key": idempotencyKey,
	})
}

// doRequest performs the actual HTTP round-trip through the circuit breaker. It returns
// the response body bytes, the HTTP status code, and any transport-level error.
// Transport errors include: circuit-breaker open, dial failures, timeouts, 5xx bodies
// that the caller has chosen to treat as transient (handled at the call site).
//
// The idempotencyKey is optional — pass "" to skip the header for read operations.
func (c *Client) doRequest(ctx context.Context, method, url string, body []byte, token, idempotencyKey string) ([]byte, int, error) {
	result, err := c.circuitBreaker.Execute(func() (any, error) {
		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}

		req, err := nethttp.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}

		if token != "" {
			// Callers pass either the raw token or "Bearer <token>". Normalize so
			// CRM always sees a well-formed header.
			if strings.HasPrefix(strings.ToLower(token), "bearer ") {
				req.Header.Set("Authorization", token)
			} else {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}

		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("execute request: %w", err)
		}
		defer resp.Body.Close()

		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("read response body: %w", readErr)
		}

		// 5xx is surfaced to the circuit breaker as an error so the breaker can
		// count it toward its failure budget, but we still return the status so
		// the caller can make a classification decision if the breaker is closed.
		if resp.StatusCode >= nethttp.StatusInternalServerError {
			return httpResult{body: respBody, status: resp.StatusCode}, fmt.Errorf("crm server error: %d", resp.StatusCode)
		}

		return httpResult{body: respBody, status: resp.StatusCode}, nil
	})

	if result == nil {
		// Circuit-breaker open, or pre-request build error — no body, no status.
		return nil, 0, err
	}

	r, ok := result.(httpResult)
	if !ok {
		return nil, 0, fmt.Errorf("crmhttp: unexpected result type %T", result)
	}

	// For 5xx the breaker returned err; surface it so the caller classifies as transient.
	if err != nil {
		return r.body, r.status, err
	}

	return r.body, r.status, nil
}

// httpResult is the payload the circuit-breaker closure returns on a non-transport
// success path. It carries both body and status so the caller can classify 4xx bodies
// without a second round-trip.
type httpResult struct {
	body   []byte
	status int
}

// classifyErrorStatus maps a non-2xx, non-404-Holder response into a business error
// sentinel. 4xx → ErrCRMBadRequest unless the body's code maps to a more specific
// sentinel (ErrIdempotencyKey, ErrAliasHolderConflict). 5xx → ErrCRMTransient.
//
// The CRM error contract is mmodel.Error (see pkg/mmodel/error.go): {code, title, message}.
// If the body fails to decode, we fall back to the status-code classification.
func (c *Client) classifyErrorStatus(span trace.Span, logger libLog.Logger, ctx context.Context, op string, status int, body []byte, fields map[string]string) error {
	crmErr := parseCRMError(body)

	// Start with the structural log, which helps operators debug client-side mapping
	// problems without having to attach a debugger.
	logFields := []libLog.Field{
		libLog.String("op", op),
		libLog.Int("status", status),
	}
	for k, v := range fields {
		logFields = append(logFields, libLog.String(k, v))
	}

	if crmErr.Code != "" {
		logFields = append(logFields, libLog.String("crm_error_code", crmErr.Code))
	}

	if status >= nethttp.StatusInternalServerError {
		businessErr := pkg.ValidateBusinessError(constant.ErrCRMTransient, constant.EntityAccountRegistration)

		libOpentelemetry.HandleSpanError(span, "CRM responded with server error", businessErr)
		logger.Log(ctx, libLog.LevelError, "CRM responded with server error", logFields...)

		return businessErr
	}

	if status == nethttp.StatusConflict {
		var mapped error

		if specific := mapCRMConflictCode(crmErr.Code); specific != nil {
			mapped = pkg.ValidateBusinessError(specific, constant.EntityAccountRegistration)
		} else {
			mapped = pkg.ValidateBusinessError(constant.ErrCRMConflict, constant.EntityAccountRegistration)
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "CRM responded with conflict", mapped)
		logger.Log(ctx, libLog.LevelWarn, "CRM responded with conflict", logFields...)

		return mapped
	}

	// Any other 4xx is a bad-request / validation failure on the CRM side.
	businessErr := pkg.ValidateBusinessError(constant.ErrCRMBadRequest, constant.EntityAccountRegistration)

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "CRM rejected request", businessErr)
	logger.Log(ctx, libLog.LevelWarn, "CRM rejected request", logFields...)

	return businessErr
}

// parseCRMError decodes a CRM error envelope; a decode failure yields a zero-value
// struct so callers can fall back to status-code-only classification.
func parseCRMError(body []byte) mmodel.Error {
	var env mmodel.Error

	if len(body) == 0 {
		return env
	}

	_ = json.Unmarshal(body, &env)

	return env
}

// mapCRMConflictCode inspects the code from a 409 CRM response body and, when it
// matches a known sentinel, returns the corresponding Ledger-side sentinel so callers
// can drive control flow with errors.Is. Unknown codes return nil, and the caller
// falls back to the generic ErrCRMConflict.
func mapCRMConflictCode(code string) error {
	switch code {
	case constant.ErrAccountAlreadyAssociated.Error(),
		constant.ErrHolderHasAliases.Error(),
		constant.ErrDocumentAssociationError.Error():
		return constant.ErrAliasHolderConflict
	case constant.ErrIdempotencyKey.Error():
		return constant.ErrIdempotencyKey
	default:
		return nil
	}
}

// Ensure at compile time that Client satisfies the port. This catches accidental
// interface drift during refactoring.
var _ CRMAccountRelationshipPort = (*Client)(nil)
