// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// recordingContext returns a context whose tracer records every ended span.
func recordingContext(t *testing.T) (context.Context, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	return libObservability.ContextWithTracer(context.Background(), tp.Tracer("tracer-client-test")), recorder
}

// endedSpan returns the ended span named name.
func endedSpan(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()

	for _, span := range recorder.Ended() {
		if span.Name() == name {
			return span
		}
	}

	require.Failf(t, "span not found", "no ended span named %q", name)

	return nil
}

func TestTracerClient_Reserve_SpanStatusByFailureClass(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantError bool
	}{
		{name: "400 rejection keeps the span out of error", status: http.StatusBadRequest},
		{name: "422 rejection keeps the span out of error", status: http.StatusUnprocessableEntity},
		{name: "500 marks the span as error", status: http.StatusInternalServerError, wantError: true},
		{name: "503 marks the span as error", status: http.StatusServiceUnavailable, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"code":"0001","title":"refused"}`))
			}))
			defer srv.Close()

			client, err := NewTracerClient(srv.URL)
			require.NoError(t, err)

			ctx, recorder := recordingContext(t)

			_, err = client.Reserve(ctx, ReserveRequest{TransactionID: fixedTransactionID})
			require.Error(t, err)

			span := endedSpan(t, recorder, "tracer.client.reserve")
			assert.Equal(t, tt.wantError, span.Status().Code == otelcodes.Error)
		})
	}
}

func TestTracerGRPCClient_Reserve_SpanStatusByFailureClass(t *testing.T) {
	tests := []struct {
		name      string
		code      codes.Code
		wantError bool
	}{
		{name: "InvalidArgument rejection keeps the span out of error", code: codes.InvalidArgument},
		{name: "FailedPrecondition rejection keeps the span out of error", code: codes.FailedPrecondition},
		{name: "Internal marks the span as error", code: codes.Internal, wantError: true},
		{name: "Unavailable marks the span as error", code: codes.Unavailable, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubReservationServer{
				reserveFn: func(*reservationv1.ReserveRequest) (*reservationv1.ReserveResult, error) {
					return nil, status.Error(tt.code, "refused")
				},
				confirmByIDFn: func(*reservationv1.ConfirmByIdRequest) (*reservationv1.ConfirmByIdResponse, error) {
					return nil, status.Error(tt.code, "refused")
				},
			}
			client := newTestGRPCClient(t, stub)

			ctx, recorder := recordingContext(t)

			_, err := client.Reserve(ctx, ReserveRequest{TransactionID: fixedTransactionID})
			require.Error(t, err)

			span := endedSpan(t, recorder, "tracer.grpc_client.reserve")
			assert.Equal(t, tt.wantError, span.Status().Code == otelcodes.Error)

			require.Error(t, client.Confirm(ctx, fixedReservationID))

			span = endedSpan(t, recorder, "tracer.grpc_client.confirm")
			assert.Equal(t, tt.wantError, span.Status().Code == otelcodes.Error)
		})
	}
}

func TestTracerClient_StatusErrorCarriesOnlyStatusAndCode(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCode    string
		mustNotShow string
	}{
		{
			name:        "problem document",
			body:        `{"type":"about:blank","title":"Bad Request","status":400,"detail":"asset SECRETVALUE is invalid","code":"0141"}`,
			wantCode:    "0141",
			mustNotShow: "SECRETVALUE",
		},
		{
			name:        "legacy envelope",
			body:        `{"code":"0141","title":"Bad Request","message":"asset SECRETVALUE is invalid"}`,
			wantCode:    "0141",
			mustNotShow: "SECRETVALUE",
		},
		{
			name:        "non-JSON body",
			body:        `upstream SECRETVALUE exploded`,
			mustNotShow: "SECRETVALUE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			client, err := NewTracerClient(srv.URL)
			require.NoError(t, err)

			_, err = client.Reserve(context.Background(), ReserveRequest{TransactionID: fixedTransactionID})
			require.Error(t, err)

			assert.Contains(t, err.Error(), "400")
			assert.NotContains(t, err.Error(), tt.mustNotShow)

			if tt.wantCode != "" {
				assert.Contains(t, err.Error(), "code "+tt.wantCode)
			} else {
				assert.NotContains(t, err.Error(), "code ")
			}
		})
	}
}
