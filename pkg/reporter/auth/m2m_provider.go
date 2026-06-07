// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/redact"
)

// maxTokenResponseBodySize is the maximum response body size for token exchange.
const maxTokenResponseBodySize = 1 * 1024 * 1024

// tokenErrorBodyLogLimit caps the number of bytes of a non-2xx token-exchange
// response body that we surface in the structured log. The body is sanitized
// by redact.SecretFields before logging, but truncation provides a second
// layer of defence: even if a future OAuth2 server echoes a large blob of
// request input we don't recognize, the log line stays bounded and
// grep-friendly. 256 bytes comfortably fits the error/error_description
// envelope returned by Casdoor and other OAuth2 servers.
const tokenErrorBodyLogLimit = 256

// tokenEndpointPath is the plugin-auth token endpoint for client_credentials grant.
const tokenEndpointPath = "/v1/login/oauth/access_token" // #nosec G101 -- URL path, not a credential

// defaultHTTPTimeout is the HTTP client timeout for token exchange requests.
const defaultHTTPTimeout = 10 * time.Second

// l2KeyPrefix is the prefix for L2 (Redis) cache keys storing M2M credentials.
const l2KeyPrefix = "m2m:cred:"

// defaultTokenCacheMargin is the safety margin applied when
// M2MProviderConfig.TokenCacheMargin is zero. Aligned with the env default
// (M2M_TOKEN_CACHE_MARGIN_SEC=60) so a provider constructed directly without
// going through bootstrap (tests, factories, future code paths) gets the same
// behavior as the production wiring — no "ghost default" surprises.
const defaultTokenCacheMargin = 60 * time.Second

// M2MCredential holds client credentials for M2M (machine-to-machine) authentication.
type M2MCredential struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// CredentialFetcher abstracts the retrieval of M2M credentials (e.g., from AWS Secrets Manager).
type CredentialFetcher interface {
	FetchCredential(ctx context.Context, tenantID, targetService string) (*M2MCredential, error)
}

// L2CredentialCache provides distributed caching for M2M credentials.
// Pass nil to use L1-only mode (in-memory).
type L2CredentialCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

// M2MProviderConfig holds configuration for the M2MCredentialProvider.
type M2MProviderConfig struct {
	// AuthAddress is the base URL of plugin-auth (e.g., "http://plugin-auth:4001").
	AuthAddress string

	// TargetService is the service name for scoping M2M credentials (e.g., "fetcher").
	TargetService string

	// CredentialTTL is how long to cache M2M credentials before re-fetching. Default: 300s.
	CredentialTTL time.Duration

	// L2Cache is the distributed cache for M2M credentials. Pass nil for L1-only mode.
	L2Cache L2CredentialCache

	// L2TTL is the TTL for L2 cache entries. Defaults to CredentialTTL if zero.
	L2TTL time.Duration

	// TokenCacheMargin is the minimum acceptable TTL on a freshly minted JWT
	// before it is considered "too close to expiry". GetToken rejects tokens
	// whose ExpiresIn is <= TokenCacheMargin. When zero, defaultTokenCacheMargin
	// (60s) is used — aligned with the env default M2M_TOKEN_CACHE_MARGIN_SEC=60
	// so a provider constructed directly (without going through bootstrap) gets
	// the same behavior as the production wiring.
	TokenCacheMargin time.Duration
}

// cachedCredential stores a fetched M2M credential with its expiry time.
type cachedCredential struct {
	credential *M2MCredential
	expiresAt  time.Time
}

// tokenExchangeResponse represents the JSON response from the plugin-auth token endpoint.
type tokenExchangeResponse struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int    `json:"expiresIn"`
}

// M2MCredentialProvider implements fetcher.M2MTokenProvider using per-tenant
// M2M credentials cached with configurable TTLs. Credentials are fetched from
// an external source (e.g., AWS Secrets Manager) and exchanged for JWT tokens
// via plugin-auth's client_credentials grant.
//
// Cache layers:
//   - L1: in-memory sync.Map (per-process, fast)
//   - L2: distributed Redis cache (optional, shared across instances)
//
// Tokens are NOT cached — each call to GetToken performs a fresh token exchange.
// The plugin-auth token endpoint is fast and caching tokens caused stale-token
// issues when the auth server restarts or tokens are revoked.
type M2MCredentialProvider struct {
	credentialCache  sync.Map // tenantID → *cachedCredential
	credentialTTL    time.Duration
	tokenCacheMargin time.Duration // Reject tokens with ExpiresIn <= this value
	targetService    string
	authAddress      string
	fetcher          CredentialFetcher
	httpClient       *http.Client
	logger           log.Logger
	tracer           trace.Tracer
	l2Cache          L2CredentialCache // L2 Redis cache (nil = L1-only mode)
	l2TTL            time.Duration     // TTL for L2 cache entries
	metrics          *M2MMetrics       // M2M credential and token metrics
}

// NewM2MCredentialProvider creates a new M2MCredentialProvider with the given
// configuration and credential fetcher.
func NewM2MCredentialProvider(
	cfg M2MProviderConfig,
	credFetcher CredentialFetcher,
	logger log.Logger,
	tracer trace.Tracer,
	metrics *M2MMetrics,
) *M2MCredentialProvider {
	l2TTL := cfg.L2TTL
	if l2TTL == 0 {
		l2TTL = cfg.CredentialTTL
	}

	if metrics == nil {
		metrics = NoopM2MMetrics()
	}

	tokenCacheMargin := cfg.TokenCacheMargin
	if tokenCacheMargin == 0 {
		tokenCacheMargin = defaultTokenCacheMargin
	}

	p := &M2MCredentialProvider{
		credentialTTL:    cfg.CredentialTTL,
		tokenCacheMargin: tokenCacheMargin,
		targetService:    cfg.TargetService,
		authAddress:      cfg.AuthAddress,
		fetcher:          credFetcher,
		httpClient:       &http.Client{Timeout: defaultHTTPTimeout},
		logger:           logger,
		tracer:           tracer,
		l2Cache:          cfg.L2Cache,
		l2TTL:            l2TTL,
		metrics:          metrics,
	}

	// G.2.8 — emit a single bootstrap log so an operator can verify, by grepping
	// pod logs alone, the effective config the provider is running with (env vars
	// only show declared values; they don't confirm the code applied them).
	logger.Log(context.Background(), log.LevelInfo, "M2M provider initialized",
		log.String("target_service", p.targetService),
		log.Int("credential_ttl_sec", int(p.credentialTTL.Seconds())),
		log.Int("token_margin_sec", int(p.tokenCacheMargin.Seconds())),
		log.Bool("l2_enabled", p.l2Cache != nil),
		log.String("auth_address", p.authAddress),
	)

	return p
}

// GetToken retrieves a fresh M2M JWT for the tenant identified in ctx.
// The token lifecycle:
//  1. Extract tenantID from context (via tmcore.GetTenantIDContext).
//  2. Get credential (cached from AWS Secrets Manager or fresh fetch).
//  3. Exchange credential for JWT via plugin-auth (client_credentials grant).
//  4. Return JWT (no caching — always fresh).
func (p *M2MCredentialProvider) GetToken(ctx context.Context) (string, error) {
	ctx, span := p.tracer.Start(ctx, "auth.m2m_provider.get_token")
	defer span.End()

	tenantID := tmcore.GetTenantIDContext(ctx)
	if tenantID == "" {
		err := fmt.Errorf("tenant ID not found in context for M2M token")
		libOtel.HandleSpanBusinessErrorEvent(span, "Missing tenant ID", err)

		return "", err
	}

	span.SetAttributes(attribute.String("app.tenant_id", tenantID))

	// Step 1: Get credential (cached or fetch from Secrets Manager)
	cred, err := p.getCredential(ctx, tenantID)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to get M2M credential", err)
		return "", fmt.Errorf("fetch M2M credential for tenant %s: %w", tenantID, err)
	}

	// Step 2: Exchange credential for JWT (always fresh)
	token, expiresIn, err := p.exchangeForToken(ctx, cred)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to exchange credential for token", err)
		return "", fmt.Errorf("exchange M2M token for tenant %s: %w", tenantID, err)
	}

	// F1.2 — Apply the token-cache safety margin. A token with TTL <= margin is
	// rejected to avoid handing the caller a JWT that may expire mid-flight.
	// expiresIn == 0 is an edge case (some Casdoor paths omit the field): treat
	// it as "TTL unknown", emit WARN, accept the token (do not block).
	marginSec := int(p.tokenCacheMargin.Seconds())

	switch {
	case expiresIn == 0:
		p.logger.Log(ctx, log.LevelWarn, "M2M token TTL unknown (expires_in=0), accepting",
			log.String("tenant_id", tenantID),
			log.String("target_service", p.targetService),
			log.Int("margin_seconds", marginSec),
		)
	case expiresIn <= marginSec:
		p.metrics.TokenTTLBelowMargin.Add(ctx, 1, p.metricAttrs(tenantID))
		p.logger.Log(ctx, log.LevelWarn, "M2M token TTL below safety margin, rejecting",
			log.String("tenant_id", tenantID),
			log.String("target_service", p.targetService),
			log.Int("expires_in_seconds", expiresIn),
			log.Int("margin_seconds", marginSec),
		)

		rejectErr := fmt.Errorf("m2m token TTL %ds below safety margin %ds", expiresIn, marginSec)
		libOtel.HandleSpanBusinessErrorEvent(span, "Token TTL below safety margin", rejectErr)

		return "", rejectErr
	}

	// G.2.4 — happy-path success log stays at DEBUG (high volume); now also
	// carries target_service and expires_in_seconds for troubleshooting.
	p.logger.Log(ctx, log.LevelDebug, "M2M token acquired",
		log.String("tenant_id", tenantID),
		log.String("target_service", p.targetService),
		log.Int("expires_in_seconds", expiresIn),
	)

	return token, nil
}

// metricAttrs returns the standard OTel attributes for M2M metric emissions.
func (p *M2MCredentialProvider) metricAttrs(tenantID string) metric.MeasurementOption {
	return metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("target_service", p.targetService),
	)
}

// l2Key builds the L2 (Redis) cache key for a tenant's M2M credential.
func l2Key(tenantID, targetService string) string {
	return l2KeyPrefix + tenantID + ":" + targetService
}

// classifyFetchError categorizes credential fetch errors for log triage.
// It enables Grafana/LogQL filtering like {error_class="secret_not_found"} to
// confirm a provisioning gap versus {error_class="auth"} for IAM problems
// versus {error_class="network"} for transient Secrets Manager issues.
// Returns one of: "secret_not_found", "network", "auth", "unknown".
func classifyFetchError(err error) string {
	if err == nil {
		return "unknown"
	}

	msg := err.Error()

	switch {
	case strings.Contains(msg, "ResourceNotFoundException"),
		strings.Contains(msg, "SecretNotFound"):
		return "secret_not_found"
	case strings.Contains(msg, "AccessDenied"),
		strings.Contains(msg, "UnauthorizedOperation"):
		return "auth"
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "connection"),
		strings.Contains(msg, "EOF"):
		return "network"
	default:
		return "unknown"
	}
}

// getCredential retrieves a cached credential using a two-level cache:
//  1. L1 (sync.Map, in-memory): fastest, per-process
//  2. L2 (Redis, optional): distributed, shared across instances
//  3. Fallback: fetch from CredentialFetcher (e.g., AWS Secrets Manager)
func (p *M2MCredentialProvider) getCredential(ctx context.Context, tenantID string) (*M2MCredential, error) {
	ctx, span := p.tracer.Start(ctx, "auth.m2m_provider.get_credential")
	defer span.End()

	attrs := p.metricAttrs(tenantID)

	// L1 key includes targetService to prevent cross-service credential leak if provider is reused.
	l1Key := tenantID + ":" + p.targetService

	// L1 check (in-memory)
	if cached, ok := p.credentialCache.Load(l1Key); ok {
		cc := cached.(*cachedCredential)
		if time.Now().Before(cc.expiresAt) {
			p.metrics.L1CacheHits.Add(ctx, 1, attrs)

			p.logger.Log(ctx, log.LevelDebug, "M2M credential L1 cache hit",
				log.String("tenant_id", tenantID),
				log.String("target_service", p.targetService),
				log.String("cache_layer", "l1"),
			)

			return cc.credential, nil
		}
	}

	// L2 check (Redis, if available)
	if p.l2Cache != nil {
		cred, err := p.getFromL2(ctx, tenantID)
		if err == nil && cred != nil {
			p.metrics.L2CacheHits.Add(ctx, 1, attrs)

			// Populate L1 from L2
			p.credentialCache.Store(l1Key, &cachedCredential{
				credential: cred,
				expiresAt:  time.Now().Add(p.credentialTTL),
			})

			p.logger.Log(ctx, log.LevelDebug, "M2M credential L2 cache hit, populated L1",
				log.String("tenant_id", tenantID),
				log.String("target_service", p.targetService),
				log.String("cache_layer", "l2"),
			)

			return cred, nil
		}
	}

	// Cache miss — fetch from Secrets Manager
	p.metrics.CacheMisses.Add(ctx, 1, attrs)

	p.logger.Log(ctx, log.LevelDebug, "M2M credential cache miss, fetching from Secrets Manager",
		log.String("tenant_id", tenantID),
		log.String("target_service", p.targetService),
		log.String("cache_layer", "sm"),
	)

	fetchStart := time.Now()

	cred, err := p.fetcher.FetchCredential(ctx, tenantID, p.targetService)
	if err != nil {
		p.metrics.FetchErrors.Add(ctx, 1, attrs)

		p.logger.Log(ctx, log.LevelError, "M2M credential fetch from Secrets Manager failed",
			log.String("tenant_id", tenantID),
			log.String("target_service", p.targetService),
			log.String("error_class", classifyFetchError(err)),
			log.Err(err),
		)

		return nil, err
	}

	if cred == nil {
		p.metrics.FetchErrors.Add(ctx, 1, attrs)

		nilErr := fmt.Errorf("credential fetcher returned nil for tenant %s", tenantID)
		p.logger.Log(ctx, log.LevelError, "M2M credential fetch from Secrets Manager failed",
			log.String("tenant_id", tenantID),
			log.String("target_service", p.targetService),
			log.String("error_class", "unknown"),
			log.Err(nilErr),
		)

		return nil, nilErr
	}

	fetchDuration := time.Since(fetchStart)
	p.metrics.FetchDuration.Record(ctx, fetchDuration.Seconds(), attrs)

	// Store in L1
	p.credentialCache.Store(l1Key, &cachedCredential{
		credential: cred,
		expiresAt:  time.Now().Add(p.credentialTTL),
	})

	// Store in L2 (if available)
	if p.l2Cache != nil {
		p.storeInL2(ctx, tenantID, cred)
	}

	p.logger.Log(ctx, log.LevelDebug, "M2M credential refreshed from Secrets Manager",
		log.String("tenant_id", tenantID),
		log.String("target_service", p.targetService),
		log.String("cache_layer", "sm"),
		log.Any("fetch_duration_ms", fetchDuration.Milliseconds()),
	)

	return cred, nil
}

// getFromL2 attempts to retrieve a credential from the L2 (Redis) cache.
func (p *M2MCredentialProvider) getFromL2(ctx context.Context, tenantID string) (*M2MCredential, error) {
	key := l2Key(tenantID, p.targetService)

	raw, err := p.l2Cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if raw == "" {
		return nil, fmt.Errorf("empty L2 cache value for key %s", key)
	}

	var cred M2MCredential
	if err := json.Unmarshal([]byte(raw), &cred); err != nil {
		return nil, fmt.Errorf("unmarshal L2 credential for key %s: %w", key, err)
	}

	return &cred, nil
}

// storeInL2 persists a credential in the L2 (Redis) cache.
func (p *M2MCredentialProvider) storeInL2(ctx context.Context, tenantID string, cred *M2MCredential) {
	key := l2Key(tenantID, p.targetService)

	data, err := json.Marshal(cred) // #nosec G117 -- ClientSecret is intentionally marshaled for Redis credential cache storage
	if err != nil {
		p.logger.Log(ctx, log.LevelError, "Failed to marshal credential for L2 cache",
			log.String("tenant_id", tenantID),
			log.String("error", err.Error()))

		return
	}

	if err := p.l2Cache.Set(ctx, key, string(data), p.l2TTL); err != nil {
		p.logger.Log(ctx, log.LevelError, "Failed to store credential in L2 cache",
			log.String("tenant_id", tenantID),
			log.String("error", err.Error()))
	}
}

// InvalidateCredentials removes cached credentials for a tenant from both L1 (sync.Map)
// and L2 (Redis) caches. It is the canonical hook the fetcher client uses after a
// downstream 401 (AUT-1003) to evict a credential that plugin-auth no longer accepts.
//
// Concurrency: safe to call from multiple goroutines concurrently. L1 uses sync.Map
// (Delete is goroutine-safe); L2 writes use the underlying L2CredentialCache contract
// (e.g., go-redis Client) which is also goroutine-safe. The operation is idempotent —
// calling it on a tenant whose cache entries do not exist is a no-op.
//
// Multi-pod note: each pod owns its own L1. Invalidating L1+L2 on one pod evicts the
// local cache and "soft-deletes" L2 (1-second TTL stub). Sibling pods detect eviction
// on their next L1 miss (which falls back to L2 → empty → fetch from Secrets Manager).
// This is by design — we accept a small propagation window in exchange for not
// needing a pub/sub fanout for credential changes.
//
// The trigger argument is a short, low-cardinality label (e.g., "unauthorized",
// "explicit", "rotation") that flows through to the OTel metric and the structured
// log so operators can attribute invalidation spikes to the call site that drove them.
//
// Errors: returns a non-nil error only when the L2 cache write fails. L1 invalidation
// is unconditional and silent (sync.Map.Delete has no error path). Callers that
// surface a downstream auth failure (e.g., a 401 on the fetcher) should log this
// error at WARN and continue — invalidation failure does not invalidate the
// original downstream error.
func (p *M2MCredentialProvider) InvalidateCredentials(ctx context.Context, tenantID, trigger string) error {
	// Normalize trigger to an allowlist before it lands in span/metric/log
	// attributes. Without this guard, a caller could pass dynamic input and
	// cause an unbounded label cardinality on the Invalidations counter
	// (Prometheus series explosion) and on the span attribute (tracing cost).
	trigger = sanitizeInvalidationTrigger(trigger)

	ctx, span := p.tracer.Start(ctx, "auth.m2m_provider.invalidate_credentials")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.tenant_id", tenantID),
		attribute.String("app.invalidation_trigger", trigger),
	)

	// Clear L1 (unconditional, no error path).
	l1Key := tenantID + ":" + p.targetService
	p.credentialCache.Delete(l1Key)

	// Clear L2 (if available). On failure, surface the error to the caller so
	// they can log/track it; do NOT abort — L1 is already cleared and the next
	// L2 hit will at worst return a stale credential that GetToken will reject.
	var l2Err error

	if p.l2Cache != nil {
		key := l2Key(tenantID, p.targetService)
		if err := p.l2Cache.Set(ctx, key, "", time.Second); err != nil {
			p.logger.Log(ctx, log.LevelError, "Failed to invalidate L2 cache",
				log.String("tenant_id", tenantID),
				log.String("target_service", p.targetService),
				log.String("trigger", trigger),
				log.Err(err),
			)

			l2Err = fmt.Errorf("invalidate L2 cache for tenant %s: %w", tenantID, err)
		}
	}

	// Increment metric AFTER L1 clear so a metric tick == a real invalidation
	// (even if L2 failed, L1 is gone). The trigger label provides attribution.
	p.metrics.Invalidations.Add(ctx, 1, p.invalidationMetricAttrs(tenantID, trigger))

	p.logger.Log(ctx, log.LevelDebug, "M2M credential invalidated",
		log.String("tenant_id", tenantID),
		log.String("target_service", p.targetService),
		log.String("trigger", trigger),
	)

	return l2Err
}

// invalidationTriggerUnknown is the sentinel value emitted when a caller
// passes a trigger string outside the recognized allowlist. Operators see
// it as a signal: "an invalidation happened from a path I haven't catalogued
// yet — go find the new caller and add it to the allowlist".
const invalidationTriggerUnknown = "unknown"

// sanitizeInvalidationTrigger maps an arbitrary caller-supplied trigger string
// to a fixed allowlist. This is the cardinality guard that keeps the
// Invalidations metric (and span attribute) bounded regardless of how careless
// future callers are with the trigger argument.
//
// Allowlist:
//   - "unauthorized" — downstream returned 401, F2 helper kicked in
//   - "explicit"     — operator/admin invoked invalidation manually (tests,
//     future admin endpoint)
//   - "rotation"     — scheduled or event-driven secret rotation
//
// Any other value (including the empty string) collapses to
// invalidationTriggerUnknown.
func sanitizeInvalidationTrigger(trigger string) string {
	switch trigger {
	case "unauthorized", "explicit", "rotation":
		return trigger
	default:
		return invalidationTriggerUnknown
	}
}

// invalidationMetricAttrs returns OTel attributes for the Invalidations counter,
// adding the low-cardinality "trigger" label so dashboards can split
// invalidation rate by reason (unauthorized vs explicit vs rotation).
// Callers MUST pass an already-normalized trigger (see sanitizeInvalidationTrigger);
// the function itself does not re-sanitize.
func (p *M2MCredentialProvider) invalidationMetricAttrs(tenantID, trigger string) metric.MeasurementOption {
	return metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("target_service", p.targetService),
		attribute.String("trigger", trigger),
	)
}

// tokenExchangeRequest represents the JSON request body for plugin-auth token exchange.
type tokenExchangeRequest struct {
	GrantType    string `json:"grantType"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// exchangeForToken performs the OAuth2 client_credentials grant against plugin-auth.
func (p *M2MCredentialProvider) exchangeForToken(ctx context.Context, cred *M2MCredential) (string, int, error) {
	ctx, span := p.tracer.Start(ctx, "auth.m2m_provider.exchange_token")
	defer span.End()

	tokenURL := p.authAddress + tokenEndpointPath

	reqBody := tokenExchangeRequest{
		GrantType:    "client_credentials",
		ClientID:     cred.ClientID,
		ClientSecret: cred.ClientSecret,
	}

	bodyJSON, err := json.Marshal(reqBody) // #nosec G117 -- ClientSecret is required in the OAuth client_credentials request body
	if err != nil {
		return "", 0, fmt.Errorf("marshal token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return "", 0, fmt.Errorf("create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Inject trace context for distributed tracing
	libOtel.InjectHTTPContext(ctx, req.Header)

	p.logger.Log(ctx, log.LevelDebug, "M2M token exchange request",
		log.String("client_id", cred.ClientID),
		log.String("target_service", p.targetService),
		log.String("auth_address", p.authAddress),
	)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Log(ctx, log.LevelError, "M2M token exchange HTTP error",
			log.String("client_id", cred.ClientID),
			log.String("target_service", p.targetService),
			log.String("auth_address", p.authAddress),
			log.Err(err),
		)

		return "", 0, fmt.Errorf("execute token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBodySize))
	if err != nil {
		return "", 0, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// M1 (security-reviewer): truncate + redact the response body before
		// logging. Some OAuth2 servers echo request input in error payloads;
		// without redaction, a non-2xx response can leak our own clientSecret
		// back into the log stream (CWE-532). The truncate-then-redact order
		// is deliberate: if a large blob slips past 256 bytes the suffix
		// "...[truncated]" never embeds itself inside what would otherwise
		// be a quoted JSON secret value.
		redactedBody := redact.SecretFields(redact.TruncateForLog(string(body), tokenErrorBodyLogLimit))

		p.logger.Log(ctx, log.LevelError, "M2M token exchange failed",
			log.String("client_id", cred.ClientID),
			log.String("target_service", p.targetService),
			log.String("auth_address", p.authAddress),
			log.Int("status_code", resp.StatusCode),
			log.String("response_body_excerpt", redactedBody),
		)

		return "", 0, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var tokenResp tokenExchangeResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", 0, fmt.Errorf("decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		p.logger.Log(ctx, log.LevelError, "M2M token exchange returned empty access token",
			log.String("client_id", cred.ClientID),
			log.String("target_service", p.targetService),
			log.Int("status_code", http.StatusOK),
			log.Int("expires_in_seconds", tokenResp.ExpiresIn),
		)

		return "", 0, fmt.Errorf("token exchange returned empty accessToken")
	}

	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}
