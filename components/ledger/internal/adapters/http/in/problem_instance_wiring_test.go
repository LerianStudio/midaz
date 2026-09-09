// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TestAssembleHumaContract_WiresTheErrorReference pins the PRODUCTION WIRING of
// the per-occurrence error reference, which nothing else does.
//
// The reference has unit tests in pkg/net/http, but they call the decorator
// themselves. With the production call removed so no caller of
// WithProblemInstance remains anywhere in the tree, every one of those tests
// still passes — the fixture supplies the very thing the fix installs. So the
// customer-facing defect could return with the suite fully green.
//
// This builds the contract through AssembleHumaContract, the seam production
// mounts, registers one failing operation on the API it returns, and reads the
// body off the wire. Remove the WithProblemInstance call from that function and
// this fails.
func TestAssembleHumaContract_WiresTheErrorReference(t *testing.T) {
	// A real SDK provider, so the request carries a genuine trace id rather than
	// the all-zero one a no-op provider produces.
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider())

	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	group := app.Group("/v2")
	group.Use(func(c fiber.Ctx) error {
		ctx, span := otel.Tracer("wiring-test").Start(c.Context(), "request")
		defer span.End()

		c.SetContext(ctx)

		return c.Next()
	})

	api := AssembleHumaContract(app, group, openapi.Config{
		Title:   "wiring-test",
		Version: "test",
		Servers: []string{"/v2"},
	})

	huma.Register(api, huma.Operation{
		OperationID: "wiringProbeNotFound",
		Method:      nethttp.MethodGet,
		Path:        "/wiring-probe",
		Summary:     "always fails",
	}, func(_ context.Context, _ *struct{}) (*struct{}, error) {
		return nil, pkgHTTP.HumaProblem(constant.ErrEntityNotFound)
	})

	resp, err := app.Test(httptest.NewRequest(nethttp.MethodGet, "/v2/wiring-probe", nil))
	require.NoError(t, err, "issue the probe request")

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body), "decode the problem body: %s", raw)

	instance, ok := body["instance"].(string)
	require.Truef(t, ok,
		"the API the production seam assembles MUST stamp the per-occurrence reference on its error bodies; got %s", raw)
	require.Lenf(t, instance, 32,
		"the reference must be the 32-character trace id a customer can quote and an operator can grep; got %q", instance)
}
