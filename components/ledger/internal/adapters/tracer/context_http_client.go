// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ContextHTTPClient uses the reservation URLs with the shared contract and
// authenticates every call with an M2M bearer token.
type ContextHTTPClient struct {
	transport *TracerClient
	config    ContextClientConfig
	tokens    TokenSource
}

// AuthorizationHeader carries the M2M bearer token on the REST seam.
const AuthorizationHeader = "Authorization"

const bearerPrefix = "Bearer "

func NewContextHTTPClient(baseURL string, config ContextClientConfig, tokens TokenSource, options ...TracerClientOption) (*ContextHTTPClient, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	if tokens == nil {
		return nil, errors.New("context REST tracer requires a token source")
	}

	transport, err := NewTracerClient(baseURL, options...)
	if err != nil {
		return nil, err
	}

	transport.headerHook = bearerHeaderHook(tokens)

	return &ContextHTTPClient{transport: transport, config: config, tokens: tokens}, nil
}

func (c *ContextHTTPClient) Reserve(ctx context.Context, request tracercontract.ReserveRequest) (_ *tracercontract.ReserveResult, retErr error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.reserve_context")
	defer span.End()
	defer func() {
		if retErr != nil {
			libOtel.HandleSpanError(span, "Context reservation failed", retErr)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, c.transport.operationTimeout)
	defer cancel()

	if err := request.Validate(ctx, c.config.Bounds); err != nil {
		return nil, err
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode context reservation: %w", err)
	}

	raw, err := c.exchange(ctx, "/v1/reservations", body, http.StatusCreated)
	if err != nil {
		return nil, err
	}

	result, err := tracercontract.DecodeReserveResultJSON(ctx, raw, c.config.MaxBodyBytes, c.config.MaxReservations)
	if err != nil {
		return nil, err
	}

	if err := result.ValidateFor(request, c.config.MaxReservations); err != nil {
		return nil, err
	}

	logger.Log(ctx, libLog.LevelDebug, "Context reservation response validated")

	return result, nil
}

func (c *ContextHTTPClient) ConfirmByTransaction(ctx context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	return c.complete(ctx, transactionID, false)
}

func (c *ContextHTTPClient) ReleaseByTransaction(ctx context.Context, transactionID uuid.UUID) (*tracercontract.TransactionCompletionResult, error) {
	return c.complete(ctx, transactionID, true)
}

func (c *ContextHTTPClient) complete(ctx context.Context, transactionID uuid.UUID, release bool) (_ *tracercontract.TransactionCompletionResult, retErr error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "tracer.complete_context")
	defer span.End()
	defer func() {
		if retErr != nil {
			libOtel.HandleSpanError(span, "Context completion failed", retErr)
		}
	}()

	if transactionID == uuid.Nil {
		return nil, constant.ErrInvalidRequestBody
	}

	ctx, cancel := context.WithTimeout(ctx, c.transport.operationTimeout)
	defer cancel()

	action, status := "confirm", "CONFIRMED"
	if release {
		action, status = "release", "RELEASED"
	}

	body, err := json.Marshal(tracercontract.CompletionRequest{ContractRevision: tracercontract.ReserveContractRevision})
	if err != nil {
		return nil, fmt.Errorf("encode context completion: %w", err)
	}

	raw, err := c.exchange(ctx, "/v1/reservations/transaction/"+transactionID.String()+"/"+action, body, http.StatusOK)
	if err != nil {
		return nil, err
	}

	result, err := tracercontract.DecodeTransactionCompletionJSON(ctx, raw, c.config.MaxBodyBytes)
	if err != nil {
		return nil, err
	}

	if result.TransactionID != transactionID || result.Status != status || result.Flipped > c.config.MaxReservations {
		return nil, constant.ErrInvalidRequestBody
	}

	logger.Log(ctx, libLog.LevelDebug, "Context completion response validated")

	return result, nil
}

// exchange posts body and returns the response when it carries
// expectedStatus. A 401 reports the rejected token and, when the token source
// can offer a different one, retries once within the same operation deadline:
// the Tracer rejected the request before evaluating it, so a retry cannot
// apply it twice. A 401 that is not retried is reported to a token source that
// can re-read its credentials. A retry that fails in transport keeps the 401 in
// its chain; a retry refused before sending is reported as that refusal alone.
func (c *ContextHTTPClient) exchange(ctx context.Context, path string, body []byte, expectedStatus int) ([]byte, error) {
	if len(body) > c.config.MaxBodyBytes {
		return nil, constant.ErrPayloadTooLarge
	}

	invalidator, canRenew := c.tokens.(TokenInvalidator)

	var rejection error

	for {
		status, raw, sentToken, err := c.post(ctx, path, body)
		if err != nil {
			// A refusal of the request, such as a tenant whose renewed
			// identity plugin-auth refused, is the answer: the earlier 401
			// must not turn it back into an outage.
			if rejection != nil && !errors.Is(err, ErrTracerRequestRejected) {
				return nil, fmt.Errorf("%w; after %w", err, rejection)
			}

			return nil, err
		}

		if status == expectedStatus {
			return raw, nil
		}

		responseErr := contextHTTPResponseError(status, raw)

		if status == http.StatusUnauthorized {
			if rejection == nil && canRenew && invalidator.Invalidate(ctx, sentToken) {
				rejection = responseErr

				continue
			}

			if rejector, ok := c.tokens.(CredentialRejector); ok {
				rejector.RejectCredentials(ctx, sentToken)
			}
		}

		return nil, responseErr
	}
}

// post performs one bounded round trip and returns the status, the body and
// the bearer token the request presented.
func (c *ContextHTTPClient) post(ctx context.Context, path string, body []byte) (int, []byte, string, error) {
	response, err := c.transport.post(ctx, path, body)
	if err != nil {
		return 0, nil, "", err
	}

	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(response.Body, int64(c.config.MaxBodyBytes)+1))
	if err != nil {
		return 0, nil, "", fmt.Errorf("%w: read response: %w", ErrTracerUnavailable, err)
	}

	if len(raw) > c.config.MaxBodyBytes {
		return 0, nil, "", fmt.Errorf("%w: response exceeds configured limit", ErrTracerUnavailable)
	}

	var sentToken string
	if response.Request != nil {
		sentToken = strings.TrimPrefix(response.Request.Header.Get(AuthorizationHeader), bearerPrefix)
	}

	return response.StatusCode, raw, sentToken, nil
}

// contextHTTPResponseError classifies a non-success status. Only a canonical
// code the seam recognizes makes a response deterministic: a refusal before
// evaluation (0043, 0487, 0537) wraps ErrTracerRequestRejected, and the other
// recognized codes keep their own meaning. Any other status of 300 or above is
// ErrTracerUnavailable whatever its value: a bare 4xx or a redirect comes from
// something other than a Tracer that evaluated the request (a mesh denial, an
// ingress default backend, a pod without the route during a rollout), so the
// fail posture and the retrier decide. A 401 additionally names the token the
// Tracer no longer accepts so the caller can renew it, and a 429 is saturation
// whatever code it carries.
func contextHTTPResponseError(status int, body []byte) error {
	if status == http.StatusUnauthorized {
		return fmt.Errorf("%w: %w: HTTP %d", ErrTracerUnavailable, constant.ErrTracerTokenUnavailable, status)
	}

	if status == http.StatusTooManyRequests {
		return fmt.Errorf("%w: HTTP %d", ErrTracerUnavailable, status)
	}

	// Never copy remote error text.
	var envelope struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		if cause := seamCause(envelope.Code); cause != nil {
			return cause
		}
	}

	if status >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: HTTP %d", ErrTracerUnavailable, status)
	}

	return fmt.Errorf("tracer contract returned HTTP %d", status)
}

func bearerHeaderHook(tokens TokenSource) func(context.Context, http.Header) error {
	return func(ctx context.Context, header http.Header) error {
		token, err := tokens.Token(ctx)
		if err != nil {
			return err
		}

		if token == "" {
			return constant.ErrTracerTokenUnavailable
		}

		header.Set(AuthorizationHeader, bearerPrefix+token)

		return nil
	}
}
