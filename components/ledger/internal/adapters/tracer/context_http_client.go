// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// ContextHTTPClient uses the existing reservation URLs with the shared contract.
// Legacy completion calls remain explicit on TracerClient.
type ContextHTTPClient struct {
	transport *TracerClient
	config    ContextClientConfig
}

func NewContextHTTPClient(baseURL string, config ContextClientConfig, options ...TracerClientOption) (*ContextHTTPClient, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	transport, err := NewTracerClient(baseURL, options...)
	if err != nil {
		return nil, err
	}

	return &ContextHTTPClient{transport: transport, config: config}, nil
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

	if err := request.Validate(ctx, c.config.Namespace, c.config.Bounds); err != nil {
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

func (c *ContextHTTPClient) exchange(ctx context.Context, path string, body []byte, expectedStatus int) ([]byte, error) {
	if len(body) > c.config.MaxBodyBytes {
		return nil, constant.ErrPayloadTooLarge
	}

	response, err := c.transport.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != expectedStatus {
		if response.StatusCode == http.StatusServiceUnavailable || response.StatusCode == http.StatusGatewayTimeout || response.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("%w: HTTP %d", ErrTracerUnavailable, response.StatusCode)
		}

		return nil, fmt.Errorf("tracer contract returned HTTP %d", response.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(response.Body, int64(c.config.MaxBodyBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %w", ErrTracerUnavailable, err)
	}

	if len(raw) > c.config.MaxBodyBytes {
		return nil, constant.ErrPayloadTooLarge
	}

	return raw, nil
}
