// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package tracer holds the ledger-side clients for the tracer service's
// two-phase reservation API (POST /v1/reservations and the per-id
// confirm/release transitions). It offers an HTTP (REST) and a gRPC transport
// behind the same TracerReserver port; the composition root selects one from
// cfg.TracerTransport. Service identity is mutual TLS (Epic 1.3), so neither
// transport carries a static shared secret; the tenant travels as a trusted
// X-Tenant-Id header / metadata (Phase 2) over the mTLS-verified connection.
package tracer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
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

	// maxErrorResponseSize limits how much of an error response body is read to
	// prevent OOM from a misconfigured or hostile upstream.
	maxErrorResponseSize = 1 << 20 // 1 MB
)

// ErrTracerUnavailable is the typed error returned when the reservation
// transport fails for an availability reason — a per-operation timeout, a
// transport error, or an open circuit breaker. The reserve anchor (F3-T13)
// branches on this with the ledger's tracer.failPosture: open proceeds
// (records SKIPPED), closed rejects. It is intentionally distinct from a
// reservation DENIED decision (a successful 201 with denied=true), which is a
// business outcome the anchor handles separately.
var ErrTracerUnavailable = errors.New("tracer reservation service unavailable")

// ReserveAccount is the account scope the tracer matches limits against. It
// serializes to the tracer's AccountContext shape ({"accountId": "..."}). The
// ledger populates AccountID with the source balance's account UUID; Type and
// Status are left empty (the ledger does not carry the tracer's card-account
// taxonomy), which the tracer treats as unconstrained optional fields.
type ReserveAccount struct {
	// AccountID is omitempty: when the ledger has no internal source account
	// (an external-only source), the account object serializes as {} rather than
	// {"accountId":""}. An empty-string accountId fails the tracer's
	// uuid.UUID parse; an absent key parses cleanly to uuid.Nil, which the
	// relaxed reserve validation accepts.
	AccountID string `json:"accountId,omitempty"`
}

// ReserveRequest is the wire body of POST /v1/reservations. It is typed
// independently of the tracer's internal model so the tracer's domain
// evolution does not leak onto the ledger's outbound contract, but its JSON
// shape is a faithful subset of the tracer's reserve contract (transactionId +
// the embedded ValidationRequest). The reserve anchor (F3-T13) populates it
// from the fee-inclusive transaction state; this client only transports it.
//
// The tracer's reserve validation requires requestId, a positive amount, a
// valid ISO-4217 currency, an in-window transactionTimestamp, and a non-nil
// account.accountId. transactionType is OPTIONAL on the reserve path (the
// ledger has no card-rail nature to honestly report; when empty the tracer
// matches account-scoped limits without a transaction-type constraint).
type ReserveRequest struct {
	TransactionID uuid.UUID      `json:"transactionId"`
	RequestID     string         `json:"requestId"`
	Amount        string         `json:"amount"`
	Currency      string         `json:"currency"`
	Account       ReserveAccount `json:"account"`
	SegmentID     string         `json:"segmentId,omitempty"`
	PortfolioID   string         `json:"portfolioId,omitempty"`
	MerchantID    string         `json:"merchantId,omitempty"`
	// TransactionType is optional on reserve. When set it must be a valid
	// tracer transaction type; the ledger leaves it empty.
	TransactionType string `json:"transactionType,omitempty"`
	// TransactionTimestamp is RFC3339; the tracer enforces a not-future /
	// not-too-far-past window against its injected clock.
	TransactionTimestamp string `json:"transactionTimestamp"`
	// LongLived hints the tracer to assign a long-lived reservation lifetime to
	// a PENDING-transaction reservation (F3-T15). It replaces the former
	// overload of transactionType=pending-long-lived, which polluted the
	// transaction-type field and broke the tracer's reserve validation.
	LongLived bool `json:"longLived,omitempty"`
}

// ReserveResult is the handle returned by a successful reserve. Denied is the
// limit-exceeded decision (no capacity held, ReservationIDs empty). Otherwise
// ReservationIDs holds one id per counter-backed limit the ledger must later
// confirm or release.
type ReserveResult struct {
	TransactionID  uuid.UUID   `json:"transactionId"`
	Denied         bool        `json:"denied"`
	ReservationIDs []uuid.UUID `json:"reservationIds"`
}

// TracerClient is the HTTP client for the tracer reservation API.
type TracerClient struct {
	baseURL          string
	httpClient       *http.Client
	operationTimeout time.Duration
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
		},
		operationTimeout: defaultOperationTimeout,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// Reserve holds limit capacity for a transaction (phase one). On a 201 it
// parses the reservation handle (including a denied=true decision, which is a
// successful response, not a transport failure). A timeout, transport error,
// or non-201 status returns an error; availability failures are
// ErrTracerUnavailable so the anchor can apply tracer.failPosture.
func (c *TracerClient) Reserve(ctx context.Context, req ReserveRequest) (*ReserveResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.client.reserve")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.transaction_id", req.TransactionID.String()))

	body, err := json.Marshal(req)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal reserve request", err)
		return nil, fmt.Errorf("marshal reserve request: %w", err)
	}

	resp, err := c.do(ctx, http.MethodPost, "/v1/reservations", body)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Reserve transport failed", err)
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		err := c.statusError("reserve", resp)
		libOpentelemetry.HandleSpanError(span, "Reserve returned unexpected status", err)

		return nil, err
	}

	var result ReserveResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode reserve response", err)
		return nil, fmt.Errorf("decode reserve response: %w", err)
	}

	logger.Log(ctx, libLog.LevelDebug, "Reservation processed",
		libLog.String("transaction_id", req.TransactionID.String()),
		libLog.Bool("denied", result.Denied),
		libLog.Int("reservations", len(result.ReservationIDs)),
	)

	return &result, nil
}

// Confirm commits a held reservation (phase two — commit). The tracer treats
// confirm as idempotent (a retry against a terminal reservation returns 200),
// so any 200 here is success.
func (c *TracerClient) Confirm(ctx context.Context, reservationID uuid.UUID) error {
	return c.transition(ctx, "confirm", reservationID)
}

// Release returns a held reservation's capacity on an aborted transaction
// (phase two — abort). Like confirm, the tracer treats release as idempotent.
func (c *TracerClient) Release(ctx context.Context, reservationID uuid.UUID) error {
	return c.transition(ctx, "release", reservationID)
}

// ConfirmByTransaction commits EVERY reservation a transaction holds (phase two
// — commit by transaction). The ledger /commit drives this with only the
// transaction id. Like the per-id confirm, the tracer treats it as idempotent
// (flipped=0 when there is nothing to confirm), so any 200 here is success.
func (c *TracerClient) ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) error {
	return c.transitionByTransaction(ctx, "confirm", transactionID)
}

// ReleaseByTransaction returns EVERY reservation a transaction holds (phase two
// — abort by transaction). The ledger /cancel drives this with only the
// transaction id. Idempotent like ConfirmByTransaction.
func (c *TracerClient) ReleaseByTransaction(ctx context.Context, transactionID uuid.UUID) error {
	return c.transitionByTransaction(ctx, "release", transactionID)
}

// transitionByTransaction is the shared by-transaction confirm/release body: POST
// the action under the /reservations/transaction/{id}/{action} path and require a
// 200. Availability failures return ErrTracerUnavailable so the caller's
// best-effort post-commit transport can swallow them (the TTL reaper backstops).
func (c *TracerClient) transitionByTransaction(ctx context.Context, action string, transactionID uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.client."+action+"_by_transaction")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.transaction_id", transactionID.String()))

	path := fmt.Sprintf("/v1/reservations/transaction/%s/%s", transactionID.String(), action)

	resp, err := c.do(ctx, http.MethodPost, path, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Reservation by-transaction transition transport failed", err)
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		err := c.statusError(action+" by transaction", resp)
		libOpentelemetry.HandleSpanError(span, "Reservation by-transaction transition returned unexpected status", err)

		return err
	}

	return nil
}

// transition is the shared confirm/release body: POST the per-id action and
// require a 200. Availability failures return ErrTracerUnavailable.
func (c *TracerClient) transition(ctx context.Context, action string, reservationID uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.client."+action)
	defer span.End()

	span.SetAttributes(attribute.String("app.request.reservation_id", reservationID.String()))

	path := fmt.Sprintf("/v1/reservations/%s/%s", reservationID.String(), action)

	resp, err := c.do(ctx, http.MethodPost, path, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Reservation transition transport failed", err)
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		err := c.statusError(action, resp)
		libOpentelemetry.HandleSpanError(span, "Reservation transition returned unexpected status", err)

		return err
	}

	return nil
}

// do executes a request against the tracer API applying the per-operation
// context timeout, the W3C trace context, and the M2M auth header. The caller
// owns the returned response body and MUST close it.
//
// Transport-availability failures (timeout, dial error) are normalised to
// ErrTracerUnavailable so the reserve anchor can branch on tracer.failPosture;
// a non-2xx status is NOT an availability failure and is surfaced verbatim by
// the caller's status check.
func (c *TracerClient) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.operationTimeout)
	defer cancel()

	var bodyReader io.Reader
	if body != nil {
		// bytes.NewReader lets http.NewRequestWithContext set req.GetBody.
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
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

	resp, err := c.httpClient.Do(req) //nolint:bodyclose // response is returned to the caller (Reserve/transition*), which owns and closes the body
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTracerUnavailable, err)
	}

	return resp, nil
}

// TenantHeader is the trusted tenant-propagation header. The tracer trusts it
// because the connection is mTLS-verified (Seam Contract: the peer is a known
// service), consistent with the durable "tenant is the trust boundary" model.
const TenantHeader = "X-Tenant-Id"

// injectTenant is the tenant-propagation injection point. mTLS replaces token
// identity, so there is no Authorization header; the tenant travels as the
// trusted X-Tenant-Id header. The ledger-side resolution of the tenant value
// from context lands in Phase 2 — this hook is the seam that wiring plugs into,
// so it is intentionally a no-op (no tenant resolution) today.
func (c *TracerClient) injectTenant(ctx context.Context, req *http.Request) {
	_ = ctx
	_ = req
}

// statusError builds the error for a non-success status. The body is read
// under a size cap for diagnostics; the message never carries request payload.
func (c *TracerClient) statusError(op string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseSize))

	return fmt.Errorf("tracer %s returned status %d: %s", op, resp.StatusCode, string(body))
}
