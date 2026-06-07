// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/redact"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Fetcher HTTP client timeout constants. Named constants avoid magic numbers
// and make per-operation timeouts discoverable.
const (
	// fetcherGlobalHTTPTimeout is the safety-net timeout on the http.Client.
	fetcherGlobalHTTPTimeout = 60 * time.Second

	// fetcherManagementTimeout is the per-operation context timeout for
	// management endpoints (list connections, get schema, validate schema).
	fetcherManagementTimeout = 10 * time.Second

	// fetcherExtractionTimeout is the per-operation context timeout for
	// creating extraction jobs (POST /v1/fetcher).
	fetcherExtractionTimeout = 30 * time.Second

	// fetcherStatusTimeout is the per-operation context timeout for
	// getting extraction job status (GET /v1/fetcher/{id}).
	fetcherStatusTimeout = 15 * time.Second

	// maxErrorResponseSize limits how much of an error response body is read
	// to prevent OOM from malicious or misconfigured upstream services.
	maxErrorResponseSize = 1 << 20 // 1 MB
)

// M2MTokenProvider provides M2M JWT tokens for inter-service auth.
// nil means single-tenant mode (no auth).
type M2MTokenProvider interface {
	GetToken(ctx context.Context) (string, error)
}

// M2MCredentialInvalidator is the optional secondary interface a token provider
// can implement to participate in the F2 defensive-401 flow. When the fetcher
// receives an HTTP 401 from a downstream call, it asks the provider to evict
// the cached credential for the calling tenant so the next request fetches
// a fresh credential from the source of truth (e.g., AWS Secrets Manager).
//
// Providers MAY implement this interface; the fetcher feature-detects it via
// type-assertion. A provider that only knows how to mint tokens (such as the
// static-secret token provider in single-tenant mode, or any test stub that
// hasn't opted in) is treated as a non-invalidator and the 401 propagates
// without an invalidation attempt.
//
// The trigger argument is the reason label that flows into the invalidation
// metric. The fetcher always passes "unauthorized" — other call sites
// (operator-initiated rotation, scheduled refresh, ...) MAY use different
// values to distinguish their attribution in dashboards.
type M2MCredentialInvalidator interface {
	InvalidateCredentials(ctx context.Context, tenantID, trigger string) error
}

// FetcherClient provides HTTP access to the Fetcher API for both management
// endpoints (list datasources, schema validation) and extraction endpoints
// (create job, get status).
type FetcherClient struct {
	baseURL     string
	httpClient  *http.Client
	m2mProvider M2MTokenProvider
	cbExecutor  CircuitBreakerExecutor
	metrics     *Metrics
}

// CircuitBreakerExecutor defines the interface for running operations through
// a circuit breaker. Matches the pkg.CircuitBreakerExecutor interface.
type CircuitBreakerExecutor interface {
	Execute(datasourceName string, fn func() (any, error)) (any, error)
}

// FetcherClientOption is a functional option for configuring FetcherClient.
type FetcherClientOption func(*FetcherClient)

// WithM2MTokenProvider configures the M2M token provider for multi-tenant auth.
// When set, every request includes an Authorization: Bearer {JWT} header.
// When nil (default), no auth headers are sent (single-tenant mode).
func WithM2MTokenProvider(provider M2MTokenProvider) FetcherClientOption {
	return func(c *FetcherClient) {
		c.m2mProvider = provider
	}
}

// WithCircuitBreaker configures the circuit breaker executor.
func WithCircuitBreaker(cb CircuitBreakerExecutor) FetcherClientOption {
	return func(c *FetcherClient) {
		c.cbExecutor = cb
	}
}

// WithHTTPClient overrides the default http.Client.
func WithHTTPClient(client *http.Client) FetcherClientOption {
	return func(c *FetcherClient) {
		c.httpClient = client
	}
}

// WithMetrics configures the OTel metrics instruments emitted by the client.
// When unset (default), a no-op Metrics is installed so call sites can emit
// without nil-guarding.
func WithMetrics(metrics *Metrics) FetcherClientOption {
	return func(c *FetcherClient) {
		if metrics != nil {
			c.metrics = metrics
		}
	}
}

// NewFetcherClient creates a new HTTP client for the Fetcher API.
// Use functional options for optional dependencies (m2mProvider, cbExecutor,
// custom httpClient, metrics).
//
// A no-op Metrics instance is installed by default so call sites can emit
// without nil-guarding. Use WithMetrics to wire real OTel instruments.
func NewFetcherClient(baseURL string, opts ...FetcherClientOption) *FetcherClient {
	c := &FetcherClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: fetcherGlobalHTTPTimeout,
		},
		metrics: NoopMetrics(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// invalidateAuthOnUnauthorized triggers M2M credential invalidation when a downstream
// response is HTTP 401. The contract is intentionally narrow:
//
//   - statusCode != 401 → no-op (every other status — including 403, 4xx, 5xx —
//     flows through the normal error path; only 401 evicts).
//   - c.m2mProvider == nil → no-op (single-tenant mode has nothing to invalidate).
//   - provider does not implement M2MCredentialInvalidator → no-op (static or
//     test providers that opt out of invalidation participate in normal auth).
//   - no tenant ID in context → no-op (we have no key to invalidate against;
//     logged at DEBUG since this is an upstream wiring issue and we don't want
//     to spam WARN on every probe call without tenancy).
//
// Errors returned by InvalidateCredentials are logged at WARN and swallowed —
// the caller still sees the original 401, which carries the actual signal an
// operator cares about. Invalidation failure (e.g., Redis transient outage)
// does not change the next request's behavior because L1 was already cleared.
func (c *FetcherClient) invalidateAuthOnUnauthorized(ctx context.Context, statusCode int) {
	if statusCode != http.StatusUnauthorized {
		return
	}

	if c.m2mProvider == nil {
		return
	}

	invalidator, ok := c.m2mProvider.(M2MCredentialInvalidator)
	if !ok {
		return
	}

	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		logger := ctxutil.NewLoggerFromContext(ctx)
		logger.Log(ctx, log.LevelDebug, "Skipping M2M invalidation on 401: tenant ID missing from context",
			log.String("trigger", "unauthorized"),
		)

		return
	}

	if err := invalidator.InvalidateCredentials(ctx, tenantID, "unauthorized"); err != nil {
		logger := ctxutil.NewLoggerFromContext(ctx)
		logger.Log(ctx, log.LevelWarn, "Failed to invalidate M2M credential after 401",
			log.String("tenant_id", tenantID),
			log.String("trigger", "unauthorized"),
			log.Err(err),
		)
	}
}

// applyAuth adds the Authorization header if an M2MTokenProvider is configured.
// In single-tenant mode (m2mProvider == nil), no headers are added.
// Per D3 decision: NO X-Organization-Id, X-API-Key, or X-Tenant-ID headers.
//
// A DEBUG log line is emitted on successful application so operators can
// correlate request/response pairs in tenancy-aware traces (G.2.6).
func (c *FetcherClient) applyAuth(ctx context.Context, req *http.Request) error {
	if c.m2mProvider == nil {
		return nil
	}

	token, err := c.m2mProvider.GetToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get M2M token: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	logger := ctxutil.NewLoggerFromContext(ctx)
	logger.Log(ctx, log.LevelDebug, "M2M auth applied to request",
		log.String("endpoint", req.URL.Path),
		log.String("tenant_id", tmcore.GetTenantIDContext(ctx)),
	)

	return nil
}

// Ping performs a readiness probe by issuing GET <baseURL>/readyz against the
// Fetcher API. The response body is intentionally ignored — only the status
// code is inspected. A 2xx status returns nil; any other outcome (including
// non-2xx, transport errors, malformed URL, context cancellation) returns
// an error.
//
// This method is used by /readyz handlers in Manager and Worker to surface
// the Fetcher's reachability without coupling them to any business endpoint.
// The caller is expected to apply a per-probe timeout via context (typical
// budget: 2 seconds).
func (c *FetcherClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/readyz", nil)
	if err != nil {
		// http.NewRequestWithContext error messages include the raw URL we
		// passed in. Redact before surfacing — the URL may carry userinfo.
		return fmt.Errorf("build ping request: %s", redact.Error(err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// httpClient.Do errors are wrapped in the form
		// `Get "URL": underlying error`. The URL string carries userinfo
		// when c.baseURL was configured with credentials. Redact before
		// surfacing — the operator should still see the underlying
		// ("connection refused", "context deadline exceeded", etc.) but
		// never the credentials.
		return fmt.Errorf("execute ping request: %s", redact.Error(err))
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// NOTE: intentionally NOT calling invalidateAuthOnUnauthorized here.
		// /readyz is unauthenticated and tenant-agnostic (no tenant ID in ctx
		// at probe time). A 401 from /readyz would indicate an upstream
		// misconfiguration, not an expired credential, so credential
		// invalidation would be both incorrect and noisy.
		return fmt.Errorf("fetcher /readyz returned %d", resp.StatusCode)
	}

	return nil
}

// doWithAuthRetry executes the request applying M2M auth. If the response is
// HTTP 401 AND the M2M provider is configured, the helper:
//
//  1. closes the first response body,
//  2. invokes invalidateAuthOnUnauthorized to evict the cached credential,
//  3. mints a fresh token via applyAuth, and
//  4. retries the request EXACTLY ONCE.
//
// If the retry also returns a non-2xx status (including another 401), the
// retry response is returned as-is so the caller's error path handles it.
// There is no second retry — the loop is bounded to a single re-attempt.
//
// In single-tenant mode (m2mProvider == nil) the helper skips the retry path
// entirely; 401 propagates verbatim to the caller.
//
// POST/PUT/PATCH bodies: the request MUST have GetBody set so the body can be
// re-streamed on retry. http.NewRequestWithContext(..., bytes.NewReader(b))
// configures GetBody automatically. GET/HEAD/DELETE without a body are
// trivially retryable.
//
// Timing budget: attempt1 (~200ms) + token mint (~500ms) + attempt2 (~200ms)
// ~= 1s in the worst case. This sits well inside every per-operation timeout
// declared above (10s / 15s / 30s) so no timeout bump is required.
//
// The caller owns the returned response body and MUST close it (matching the
// existing httpClient.Do contract).
func (c *FetcherClient) doWithAuthRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)

	if err := c.applyAuth(ctx, req); err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Fast path: not 401 OR no provider to re-auth against.
	if resp.StatusCode != http.StatusUnauthorized || c.m2mProvider == nil {
		return resp, nil
	}

	// 401 with an M2M provider → invalidate, retry once.
	_ = resp.Body.Close()

	tracer := ctxutil.NewTracerFromContext(ctx)

	retryCtx, span := tracer.Start(ctx, "fetcher.client.auth_retry")
	defer span.End()

	tenantID := tmcore.GetTenantIDContext(retryCtx)
	endpoint := req.URL.Path

	span.SetAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.Int("original_status_code", http.StatusUnauthorized),
	)

	logger.Log(retryCtx, log.LevelWarn, "Fetcher returned 401, invalidating credential and retrying",
		log.String("endpoint", endpoint),
		log.String("tenant_id", tenantID),
		log.Int("attempt", 1),
	)

	c.invalidateAuthOnUnauthorized(retryCtx, http.StatusUnauthorized)

	retryReq, cloneErr := cloneRequestForRetry(req, retryCtx)
	if cloneErr != nil {
		c.metrics.AuthRetries.Add(retryCtx, 1, metric.WithAttributes(
			attribute.String("tenant_id", tenantID),
			attribute.String("endpoint", endpoint),
			attribute.String("outcome", "failure"),
		))
		libOpentelemetry.HandleSpanError(span, "Failed to clone request for auth retry", cloneErr)

		return nil, fmt.Errorf("clone request for retry: %w", cloneErr)
	}

	if err := c.applyAuth(retryCtx, retryReq); err != nil {
		c.metrics.AuthRetries.Add(retryCtx, 1, metric.WithAttributes(
			attribute.String("tenant_id", tenantID),
			attribute.String("endpoint", endpoint),
			attribute.String("outcome", "failure"),
		))
		libOpentelemetry.HandleSpanError(span, "Failed to re-apply auth on retry", err)

		return nil, fmt.Errorf("reauth after 401: %w", err)
	}

	retryResp, err := c.httpClient.Do(retryReq)
	if err != nil {
		c.metrics.AuthRetries.Add(retryCtx, 1, metric.WithAttributes(
			attribute.String("tenant_id", tenantID),
			attribute.String("endpoint", endpoint),
			attribute.String("outcome", "failure"),
		))
		libOpentelemetry.HandleSpanError(span, "Auth retry request failed", err)

		return nil, err
	}

	outcome := "failure"
	if retryResp.StatusCode >= 200 && retryResp.StatusCode < 300 {
		outcome = "success"

		logger.Log(retryCtx, log.LevelDebug, "Fetcher auth retry succeeded with fresh token",
			log.String("endpoint", endpoint),
			log.String("tenant_id", tenantID),
			log.Int("attempt", 2),
			log.Int("status_code", retryResp.StatusCode),
		)
	} else {
		logger.Log(retryCtx, log.LevelError, "Fetcher auth retry failed, propagating",
			log.String("endpoint", endpoint),
			log.String("tenant_id", tenantID),
			log.Int("attempt", 2),
			log.Int("status_code", retryResp.StatusCode),
		)
	}

	span.SetAttributes(
		attribute.Int("retry_status_code", retryResp.StatusCode),
		attribute.String("retry_outcome", outcome),
	)

	c.metrics.AuthRetries.Add(retryCtx, 1, metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("endpoint", endpoint),
		attribute.String("outcome", outcome),
	))

	return retryResp, nil
}

// cloneRequestForRetry duplicates an HTTP request preserving its body. For
// methods with a body (POST/PUT/PATCH), req.GetBody MUST be set by the caller
// (which http.NewRequestWithContext does automatically when given a
// *bytes.Reader, *bytes.Buffer, or *strings.Reader). If GetBody is nil on a
// request that carries a body, retry is impossible and an error is returned.
// For GET/HEAD/DELETE without a body, GetBody is optional.
//
// The retry context is threaded through req.Clone so span/logger lineage is
// preserved on attempt #2.
func cloneRequestForRetry(req *http.Request, ctx context.Context) (*http.Request, error) {
	newReq := req.Clone(ctx)

	if req.Body == nil {
		return newReq, nil
	}

	if req.GetBody == nil {
		return nil, fmt.Errorf("request body without GetBody — cannot retry")
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("get retryable body: %w", err)
	}

	newReq.Body = body

	return newReq, nil
}
