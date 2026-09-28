// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tracer holds the ledger-side clients for the tracer service's
// contextual reservation API (POST /v1/reservations and the by-transaction
// confirm/release completions). It offers an HTTP (REST) and a gRPC transport
// behind the same contextual port; the composition root selects one from
// cfg.TracerTransport. The tenant travels as a trusted X-Tenant-Id header /
// metadata; the REST client adds an M2M bearer token and the gRPC client
// identifies itself with its mTLS certificate.
package tracer

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
)

// Tracer reservation client timeout constants. The global timeout is the
// http.Client safety net; the per-operation timeout is the budget the reserve
// anchor (F3-T13) gates the request on and is overridable from the ledger's
// tracer.timeoutMs setting (F3-T10).
const (
	// defaultGlobalHTTPTimeout is the safety-net timeout on the http.Client.
	defaultGlobalHTTPTimeout = 30 * time.Second

	// defaultOperationTimeout is the per-operation context timeout applied when
	// the caller does not configure one via WithOperationTimeout. It mirrors the
	// tracer.timeoutMs default (250ms) so a misconfigured client still fails
	// fast rather than holding the transaction create path open.
	defaultOperationTimeout = 250 * time.Millisecond
)

// ErrTracerUnavailable is the typed error returned when the reservation
// transport fails for an availability reason — a per-operation timeout, a
// transport error, or an open circuit breaker. The reserve anchor (F3-T13)
// branches on this with the ledger's tracer.failPosture: open proceeds
// (records SKIPPED), closed rejects. It is intentionally distinct from a DENY
// or REVIEW decision, which is a business outcome the anchor handles separately.
var ErrTracerUnavailable = errors.New("tracer reservation service unavailable")

// TracerClient is the HTTP transport under ContextHTTPClient: timeout, trace
// context, tenant header and the optional header hook.
type TracerClient struct {
	baseURL          string
	httpClient       *http.Client
	operationTimeout time.Duration
	// headerHook adds request headers after the tenant; its error aborts the
	// request before dialing. Header values it sets are never logged.
	headerHook func(ctx context.Context, header http.Header) error
}

// TracerClientOption configures a TracerClient.
type TracerClientOption func(*TracerClient)

// WithOperationTimeout sets the per-operation context timeout from the ledger's
// tracer.timeoutMs setting. A non-positive value leaves the default in place.
func WithOperationTimeout(d time.Duration) TracerClientOption {
	return func(c *TracerClient) {
		if d > 0 {
			c.operationTimeout = d
		}
	}
}

// WithTLSConfig secures the REST seam with mutual TLS (Epic 1.3): it installs an
// http.Transport carrying the supplied *tls.Config, which presents the ledger's
// client certificate and verifies the tracer's server certificate. A nil config
// leaves the default plaintext transport (mesh mode, where a sidecar originates
// mTLS). The composition root builds the config from TRACER_TLS_* and only
// passes it in mtls mode.
func WithTLSConfig(tlsConfig *tls.Config) TracerClientOption {
	return func(c *TracerClient) {
		if tlsConfig != nil {
			c.httpClient.Transport = &http.Transport{TLSClientConfig: tlsConfig}
		}
	}
}

// NewTracerClient builds an HTTP client for the tracer reservation API.
// Optional dependencies (operation timeout) are supplied via functional
// options. It returns an error when baseURL is empty so a misconfigured
// composition root fails at boot rather than at the first transaction.
func NewTracerClient(baseURL string, opts ...TracerClientOption) (*TracerClient, error) {
	if baseURL == "" {
		return nil, errors.New("empty baseURL passed to NewTracerClient")
	}

	c := &TracerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultGlobalHTTPTimeout,
			// A redirect is answered as the response itself: following it would
			// replay the bearer token and the tenant to another location.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		operationTimeout: defaultOperationTimeout,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// post executes a POST request against the tracer API applying the per-operation
// context timeout, the W3C trace context, the tenant header and the optional
// header hook. The caller
// owns the returned response body and MUST close it.
//
// Transport-availability failures (timeout, dial error, header hook failure)
// are normalised to ErrTracerUnavailable so the reserve anchor can branch on
// tracer.failPosture; a non-2xx status is NOT an availability failure and is surfaced verbatim by
// the caller's status check.
func (c *TracerClient) post(ctx context.Context, path string, body []byte) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)

	var bodyReader io.Reader
	if body != nil {
		// bytes.NewReader lets http.NewRequestWithContext set req.GetBody.
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bodyReader)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build tracer request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Propagate the W3C trace context so the tracer's otelfiber middleware
	// continues the ledger transaction-create trace instead of starting a fresh
	// root span per reserve/confirm/release.
	libOpentelemetry.InjectHTTPContext(ctx, req.Header)

	c.injectTenant(ctx, req)

	if c.headerHook != nil {
		if err := c.headerHook(ctx, req.Header); err != nil {
			cancel()
			return nil, fmt.Errorf("%w: %w", ErrTracerUnavailable, err)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("%w: %w", ErrTracerUnavailable, err)
	}

	resp.Body = &reservationResponseBody{ReadCloser: resp.Body, cancel: cancel}

	return resp, nil
}

// reservationResponseBody keeps the operation deadline active while the caller
// decodes the response. Closing the body releases its timer and request context.
type reservationResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *reservationResponseBody) Close() error {
	defer b.cancel()
	return b.ReadCloser.Close()
}

// TenantHeader is the trusted tenant-propagation header. The tracer trusts it
// only from an authenticated service peer.
const TenantHeader = "X-Tenant-Id"

// tenantMetadataKey is the gRPC outgoing-metadata key for the same trusted
// tenant value the REST transport carries in TenantHeader. gRPC metadata keys
// are lower-cased, so it is derived from TenantHeader to keep the REST header
// and the gRPC metadata key from drifting apart.
var tenantMetadataKey = strings.ToLower(TenantHeader)

// injectTenant propagates the request's tenant to the tracer as the trusted
// X-Tenant-Id header. The value is resolved from context via tmcore.GetTenantIDContext;
// in single-tenant mode it is empty and no header is set (the tracer then runs
// its single-tenant pass-through). The tenant value is never logged.
func (c *TracerClient) injectTenant(ctx context.Context, req *http.Request) {
	if tenant := tmcore.GetTenantIDContext(ctx); tenant != "" {
		req.Header.Set(TenantHeader, tenant)
	}
}
