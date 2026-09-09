// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// =============================================================================
// The per-occurrence error reference (RFC 9457 `instance`).
//
// A Midaz error body must carry a reference the customer can quote to support,
// and it has to be the same string an operator greps in the logs. These tests
// drive real requests through both transports, decode the bodies, and compare
// the published reference against the trace id read through the accessor the
// loggers use.
// =============================================================================

// loggerTraceID is the accessor every Midaz logger uses to emit its trace id
// field (see components/tracer/pkg/logging.WithTrace and
// pkg/net/http/fiber_error_handler.go). Reproduced here rather than imported so
// the test compares the published reference against the LOGGING side
// independently, instead of against the helper under test.
func loggerTraceID(ctx context.Context) string {
	span := oteltrace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return ""
	}

	return span.SpanContext().TraceID().String()
}

// recordSpans installs a real SDK tracer provider for the test, so requests
// carry genuine trace ids from the default generator rather than a stub.
func recordSpans(t *testing.T) {
	t.Helper()

	prev := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(tracetest.NewSpanRecorder()))

	otel.SetTracerProvider(tp)

	t.Cleanup(func() {
		otel.SetTracerProvider(prev)

		if err := tp.Shutdown(context.Background()); err != nil {
			t.Logf("shutdown tracer provider: %v", err)
		}
	})
}

// errorForStatus returns a canonical Midaz error for each status class the
// error envelope emits, so the reference is asserted across the range rather
// than on one status.
func errorForStatus() map[int]error {
	return map[int]error{
		http.StatusBadRequest:          pkg.ValidateBusinessError(constant.ErrInvalidSortOrder, "Limit"),
		http.StatusNotFound:            pkg.ValidateBusinessError(constant.ErrLimitNotFound, "Limit"),
		http.StatusConflict:            pkg.ValidateBusinessError(constant.ErrLimitNameAlreadyExists, "Limit"),
		http.StatusInternalServerError: pkg.ValidateInternalError(errors.New("boom"), "Limit"),
	}
}

// tracedApp builds a Fiber app whose /v1 group starts a real span per request,
// exactly as the observability middleware does in production, and returns the
// Huma API wrapped the way both components wrap theirs. traced reports the
// trace id the logger would print for the request, captured inside the handler.
func tracedApp(t *testing.T, withSpan bool) (*fiber.App, huma.API, *string) {
	t.Helper()

	libProblem.Install()

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	loggedID := ""

	group := f.Group("/v1")
	group.Use(func(c fiber.Ctx) error {
		if withSpan {
			ctx, span := otel.Tracer("test").Start(c.Context(), "request")
			defer span.End()

			c.SetContext(ctx)
		}

		loggedID = loggerTraceID(c.Context())

		return c.Next()
	})

	api := pkgHTTP.WithProblemInstance(
		openapi.New(f, group, openapi.Config{Title: "instance-test", Version: "test", Servers: []string{"/v1"}}),
	)

	return f, api, &loggedID
}

func decodeBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body")
	require.NoError(t, resp.Body.Close())

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body), "decode problem body: %s", raw)

	return body
}

// TestProblemInstance_HumaTransport_PublishesTheLoggedTraceID covers the Huma
// transport, which is how 378 handler error paths answer. Every status class
// must carry the reference, and it must equal the trace id the logger writes for
// the same request.
func TestProblemInstance_HumaTransport_PublishesTheLoggedTraceID(t *testing.T) {
	recordSpans(t)

	for status, opErr := range errorForStatus() {
		status, opErr := status, opErr

		t.Run(http.StatusText(status), func(t *testing.T) {
			f, api, logged := tracedApp(t, true)

			huma.Register(api, huma.Operation{
				OperationID: "boom",
				Method:      http.MethodGet,
				Path:        "/boom",
				Summary:     "always fails",
			}, func(_ context.Context, _ *struct{}) (*struct{}, error) {
				return nil, pkgHTTP.HumaProblem(opErr)
			})

			resp, err := f.Test(httptest.NewRequest(http.MethodGet, "/v1/boom", nil))
			require.NoError(t, err, "issue request")
			require.Equal(t, status, resp.StatusCode, "the status class under test")

			body := decodeBody(t, resp)

			instance, ok := body["instance"].(string)
			require.True(t, ok, "the error body MUST carry an instance reference; got %v", body)
			require.Len(t, instance, 32, "the reference is a 32-character trace id")
			require.NotEmpty(t, *logged, "the request must have carried a trace")
			require.Equal(t, *logged, instance,
				"the published reference MUST be byte-for-byte the trace id the logger writes, or a customer quoting it reaches nothing")
		})
	}
}

// TestProblemInstance_FiberTransport_PublishesTheLoggedTraceID covers the Fiber
// transport (WithError), which does not pass through Huma's response transform
// and therefore stamps the reference at its own construction point.
func TestProblemInstance_FiberTransport_PublishesTheLoggedTraceID(t *testing.T) {
	recordSpans(t)

	for status, opErr := range errorForStatus() {
		status, opErr := status, opErr

		t.Run(http.StatusText(status), func(t *testing.T) {
			f, _, logged := tracedApp(t, true)

			f.Get("/v1/fiber-boom", func(c fiber.Ctx) error {
				return pkgHTTP.WithError(c, opErr)
			})

			resp, err := f.Test(httptest.NewRequest(http.MethodGet, "/v1/fiber-boom", nil))
			require.NoError(t, err, "issue request")
			require.Equal(t, status, resp.StatusCode, "the status class under test")

			body := decodeBody(t, resp)

			instance, ok := body["instance"].(string)
			require.True(t, ok, "the Fiber error body MUST carry an instance reference too; got %v", body)
			require.Equal(t, *logged, instance,
				"both transports MUST publish the same reference the logger writes")
		})
	}
}

// TestProblemInstance_AbsentWhenTheRequestHasNoTrace is the shape assertion an
// empty string would break. With no trace there is no reference, and the member
// must be MISSING from the body — a present-but-blank value reads as an answer
// and sends support chasing it.
func TestProblemInstance_AbsentWhenTheRequestHasNoTrace(t *testing.T) {
	f, api, logged := tracedApp(t, false)

	huma.Register(api, huma.Operation{
		OperationID: "boom-untraced",
		Method:      http.MethodGet,
		Path:        "/boom-untraced",
		Summary:     "always fails",
	}, func(_ context.Context, _ *struct{}) (*struct{}, error) {
		return nil, pkgHTTP.HumaProblem(pkg.ValidateBusinessError(constant.ErrLimitNotFound, "Limit"))
	})

	resp, err := f.Test(httptest.NewRequest(http.MethodGet, "/v1/boom-untraced", nil))
	require.NoError(t, err, "issue request")

	body := decodeBody(t, resp)

	require.Empty(t, *logged, "the fixture must not have started a span")

	_, present := body["instance"]
	require.False(t, present,
		"with no trace the instance member MUST be absent, not blank; body was %v", body)
}

// TestTraceReference_MatchesTheLoggingAccessor pins the helper itself against the
// logging accessor for a traced and an untraced context, which is the property
// the greppability of the reference rests on.
func TestTraceReference_MatchesTheLoggingAccessor(t *testing.T) {
	recordSpans(t)

	ctx, span := otel.Tracer("test").Start(context.Background(), "op")
	defer span.End()

	require.Equal(t, loggerTraceID(ctx), pkgHTTP.TraceReference(ctx),
		"the reference and the logged trace id MUST come from the same accessor")
	require.Len(t, pkgHTTP.TraceReference(ctx), 32, "32 hex characters from the SDK's default generator")

	require.Empty(t, pkgHTTP.TraceReference(context.Background()), "no span means no reference")
	require.Empty(t, pkgHTTP.TraceReference(nil), "a nil context must not panic")
}
