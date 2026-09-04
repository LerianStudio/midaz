// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	obsconst "github.com/LerianStudio/lib-observability/v4/constants"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	libObsMiddleware "github.com/LerianStudio/lib-observability/v4/middleware"
	"github.com/LerianStudio/lib-observability/v4/tracing"
	midazhttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Names owned by lib-observability; unexported there, so they are restated
// here. A rename upstream must fail this test rather than pass silently.
const (
	tenantRequestsMetric = "lerian.http.server.requests.by_tenant"
	tenant4xxMetric      = "lerian.http.server.responses_4xx.by_tenant"
	tenant5xxMetric      = "lerian.http.server.responses_5xx.by_tenant"
	tenantLatencyMetric  = "lerian.http.server.latency.by_tenant"
)

// TestTenantHTTPMetrics_LabelledFromJWTClaim wires the two halves that production
// wires — the ledger's telemetry middleware and the auth-assertion middleware —
// around a real OpenTelemetry meter, and asserts the per-tenant series carry the
// tenant identity from the JWT. Each half is inert alone: the middleware records
// nothing without an attestation, and the assertion attests into a context
// nothing reads. Only the pair produces a labelled series.
func TestTenantHTTPMetrics_LabelledFromJWTClaim(t *testing.T) {
	t.Parallel()

	tenantID := uuid.MustParse("0f6e2b3a-1c4d-4e5f-8a9b-0c1d2e3f4a5b")

	cases := []struct {
		name       string
		status     int
		wantMetric string
	}{
		{name: "success counts the request", status: fiber.StatusOK, wantMetric: tenantRequestsMetric},
		{name: "client error counts 4xx", status: fiber.StatusNotFound, wantMetric: tenant4xxMetric},
		{name: "server error counts 5xx", status: fiber.StatusInternalServerError, wantMetric: tenant5xxMetric},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reader := sdkmetric.NewManualReader()
			app := newTenantMetricsApp(t, reader, tc.status)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", mustUnsignedToken(t, jwt.MapClaims{
				"sub":        "user-1",
				"tenantId":   tenantID.String(),
				"tenantSlug": "acme",
			})))

			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tc.status, resp.StatusCode)

			attrs := requireSingleSumDataPoint(t, reader, tc.wantMetric)
			assertTenantAttrs(t, attrs, tenantID, "acme")

			// The latency histogram is recorded on every attested request,
			// whatever the status, so it is asserted alongside each counter.
			require.NotNil(t, findHistogram(t, reader, tenantLatencyMetric),
				"%s missing", tenantLatencyMetric)
		})
	}
}

// TestTenantHTTPMetrics_NotEmittedWithoutUUIDClaim locks the else branch: a
// tenant claim that passes tmcore validation but is not a UUID must leave the
// per-tenant series absent rather than emit an unlabelled or forged identity.
func TestTenantHTTPMetrics_NotEmittedWithoutUUIDClaim(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		claims jwt.MapClaims
	}{
		{name: "non-uuid tenant", claims: jwt.MapClaims{"sub": "u", "tenantId": "org_01KHVKQQP6D2N4RDJK0ADEKQX1"}},
		{name: "no tenant claim", claims: jwt.MapClaims{"sub": "u"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reader := sdkmetric.NewManualReader()
			app := newTenantMetricsApp(t, reader, fiber.StatusOK)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", mustUnsignedToken(t, tc.claims)))

			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, fiber.StatusOK, resp.StatusCode)

			for _, name := range []string{tenantRequestsMetric, tenant4xxMetric, tenant5xxMetric, tenantLatencyMetric} {
				assert.Nil(t, findMetric(t, reader, name), "%s must not be emitted", name)
			}

			// The global RED metric is unconditional: no attestation must not
			// mean no telemetry.
			require.NotNil(t, findMetric(t, reader, "http.server.request.duration"))
		})
	}
}

func newTenantMetricsApp(t *testing.T, reader *sdkmetric.ManualReader, status int) *fiber.App {
	t.Helper()

	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	factory, err := metrics.NewMetricsFactory(meterProvider.Meter("test"), nil)
	require.NoError(t, err)

	telemetry := &tracing.Telemetry{
		TelemetryConfig: tracing.TelemetryConfig{
			LibraryName:     "midaz-test",
			ServiceName:     "midaz-test",
			EnableTelemetry: true,
		},
		TracerProvider: sdktrace.NewTracerProvider(),
		MeterProvider:  meterProvider,
		MetricsFactory: factory,
	}

	app := fiber.New()
	app.Use(libObsMiddleware.NewTelemetryMiddleware(telemetry).WithAuthenticatedTenantHTTPMetrics(telemetry))

	chain := midazhttp.ProtectedRouteChain(
		func(c fiber.Ctx) error { return c.Next() },
		&midazhttp.ProtectedRouteOptions{
			PostAuthMiddlewares: []fiber.Handler{midazhttp.MarkTrustedAuthAssertion()},
		},
		func(c fiber.Ctx) error { return c.SendStatus(status) },
	)

	tail := make([]any, len(chain)-1)
	for i, h := range chain[1:] {
		tail[i] = h
	}

	app.Get("/test", chain[0], tail...)

	return app
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) *metricdata.ResourceMetrics {
	t.Helper()

	rm := &metricdata.ResourceMetrics{}
	require.NoError(t, reader.Collect(context.Background(), rm))

	return rm
}

func findMetric(t *testing.T, reader *sdkmetric.ManualReader, name string) *metricdata.Metrics {
	t.Helper()

	for _, sm := range collect(t, reader).ScopeMetrics {
		for i, m := range sm.Metrics {
			if m.Name == name {
				return &sm.Metrics[i]
			}
		}
	}

	return nil
}

func findHistogram(t *testing.T, reader *sdkmetric.ManualReader, name string) *metricdata.Histogram[float64] {
	t.Helper()

	m := findMetric(t, reader, name)
	if m == nil {
		return nil
	}

	h, ok := m.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "expected float64 histogram for %s, got %T", name, m.Data)

	return &h
}

func requireSingleSumDataPoint(t *testing.T, reader *sdkmetric.ManualReader, name string) attribute.Set {
	t.Helper()

	m := findMetric(t, reader, name)
	require.NotNil(t, m, "%s not recorded", name)

	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "expected int64 sum for %s, got %T", name, m.Data)
	require.Len(t, sum.DataPoints, 1)
	require.Equal(t, int64(1), sum.DataPoints[0].Value)

	return sum.DataPoints[0].Attributes
}

func assertTenantAttrs(t *testing.T, attrs attribute.Set, tenantID uuid.UUID, name string) {
	t.Helper()

	got, ok := attrs.Value(attribute.Key(obsconst.AttrKeyTenantID))
	require.True(t, ok, "%s attribute missing", obsconst.AttrKeyTenantID)
	assert.Equal(t, tenantID.String(), got.AsString())

	gotName, ok := attrs.Value(attribute.Key(obsconst.AttrKeyTenantName))
	require.True(t, ok, "%s attribute missing", obsconst.AttrKeyTenantName)
	assert.Equal(t, name, gotName.AsString())
}

func mustUnsignedToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()

	signed, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	return signed
}

// TestTenantSpanAttribute_SeededForEveryAuthenticatedRequest proves the trace
// half. lib-commons' tenant middleware writes the same baggage member, but it is
// absent from the metadata-index chain, so this covers the assertion middleware
// on its own — the configuration a route without tenant-DB resolution runs in.
//
// Unlike the metrics path, this one carries a non-UUID tenant too: baggage takes
// the claim verbatim, while the metric attestation requires a parsed UUID.
func TestTenantSpanAttribute_SeededForEveryAuthenticatedRequest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		tenantID string
		want     string
	}{
		{name: "uuid tenant", tenantID: "0f6e2b3a-1c4d-4e5f-8a9b-0c1d2e3f4a5b", want: "0f6e2b3a-1c4d-4e5f-8a9b-0c1d2e3f4a5b"},
		{name: "legacy non-uuid tenant", tenantID: "org_01KHVKQQP6D2N4RDJK0ADEKQX1", want: "org_01KHVKQQP6D2N4RDJK0ADEKQX1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			exporter := tracetest.NewInMemoryExporter()
			tracerProvider := sdktrace.NewTracerProvider(
				sdktrace.WithSpanProcessor(tracing.RedactingAttrBagSpanProcessor{}),
				sdktrace.WithSyncer(exporter),
			)

			app := fiber.New()

			chain := midazhttp.ProtectedRouteChain(
				func(c fiber.Ctx) error { return c.Next() },
				&midazhttp.ProtectedRouteOptions{
					PostAuthMiddlewares: []fiber.Handler{midazhttp.MarkTrustedAuthAssertion()},
				},
				func(c fiber.Ctx) error {
					// Stands in for any use case opening a span behind the chain.
					_, span := tracerProvider.Tracer("test").Start(c.Context(), "app.work")
					span.End()

					return c.SendStatus(fiber.StatusNoContent)
				},
			)

			tail := make([]any, len(chain)-1)
			for i, h := range chain[1:] {
				tail[i] = h
			}

			app.Get("/test", chain[0], tail...)

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", mustUnsignedToken(t, jwt.MapClaims{
				"sub":      "user-1",
				"tenantId": tc.tenantID,
			})))

			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, fiber.StatusNoContent, resp.StatusCode)

			spans := exporter.GetSpans()
			require.Len(t, spans, 1)

			var got string

			for _, attr := range spans[0].Attributes {
				if attr.Key == attribute.Key(obsconst.AttrKeyTenantID) {
					got = attr.Value.AsString()
				}
			}

			assert.Equal(t, tc.want, got, "%s missing from the application span", obsconst.AttrKeyTenantID)
		})
	}
}
