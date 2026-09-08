// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	"github.com/LerianStudio/midaz/v4/pkg"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// TestEnvelopeVersionBoundary is the load-bearing assertion of this change: the
// error envelope is a function of the route version and of NOTHING else.
//
// It builds the same routes twice — once with ErrorEnvelope in the chain, once
// without — and compares. /v1 must differ (that is the point); every other prefix
// must be byte-identical, because a middleware that quietly touched /v2 would be
// indistinguishable from one that touched it deliberately.
func TestEnvelopeVersionBoundary(t *testing.T) {
	withMiddleware := buildEnvelopeProbeApp(t, true)
	withoutMiddleware := buildEnvelopeProbeApp(t, false)

	unchanged := []struct {
		name string
		path string
	}{
		{name: "v2 invalid path parameter", path: "/v2/organizations/not-a-uuid"},
		{name: "v2 not found", path: "/v2/probe/not-found"},
		{name: "v2 field validation", path: "/v2/probe/fields"},
		{name: "v2 internal error", path: "/v2/probe/boom"},
		{name: "v2 unprocessable", path: "/v2/probe/unprocessable"},
		{name: "unversioned route", path: "/probe/boom"},
		{name: "v1 success carries no error envelope at all", path: "/v1/probe/ok"},
	}

	for _, testCase := range unchanged {
		t.Run("unchanged: "+testCase.name, func(t *testing.T) {
			gotStatus, gotBody, gotType := driveProbe(t, withMiddleware, testCase.path)
			wantStatus, wantBody, wantType := driveProbe(t, withoutMiddleware, testCase.path)

			assert.Equal(t, wantStatus, gotStatus)
			assert.Equal(t, wantBody, gotBody, "only /v1 may change shape")
			assert.Equal(t, wantType, gotType, "only /v1 may change media type")
		})
	}

	rewritten := []struct {
		name string
		path string
		want string
	}{
		{
			// The bug report, end to end. entityType is dropped because v3 dropped
			// it for a plain ValidationError.
			name: "v1 invalid path parameter is the v3 envelope",
			path: "/v1/organizations/not-a-uuid",
			want: `{"title":"Invalid Path Parameter","message":"One or more path parameters are in an ` +
				`incorrect format. Please check the following parameters organization_id and ensure they ` +
				`meet the required format before trying again.","code":"0065"}`,
		},
		{
			name: "v1 not found is the flat envelope without entityType",
			path: "/v1/probe/not-found",
			want: `{"code":"0007","message":"No entity was found for the given ID.","title":"Entity Not Found"}`,
		},
		{
			name: "v1 field validation keeps entityType and fields",
			path: "/v1/probe/fields",
			want: `{"entityType":"Account","title":"Missing Fields in Request",` +
				`"message":"Your request is missing fields.","code":"0018",` +
				`"fields":{"name":"name is required"}}`,
		},
		{
			name: "v1 unprocessable is the flat envelope",
			path: "/v1/probe/unprocessable",
			want: `{"code":"0019","message":"The operation cannot be completed.","title":"Unprocessable Operation"}`,
		},
		{
			// Both changes visible at once: the v3 shape, and the registry text the
			// scrub used to replace with "internal error".
			name: "v1 internal error carries the registry text in the v3 shape",
			path: "/v1/probe/boom",
			want: `{"code":"0046","message":"The server encountered an unexpected error. Please try again ` +
				`later or contact support.","title":"Internal Server Error"}`,
		},
	}

	for _, testCase := range rewritten {
		t.Run("rewritten: "+testCase.name, func(t *testing.T) {
			_, gotBody, gotType := driveProbe(t, withMiddleware, testCase.path)

			assert.Equal(t, testCase.want, gotBody)
			assert.Contains(t, gotType, fiber.MIMEApplicationJSON)
			assert.NotContains(t, gotType, "problem+json")

			_, baselineBody, _ := driveProbe(t, withoutMiddleware, testCase.path)
			assert.NotEqual(t, baselineBody, gotBody, "this case exists to prove /v1 DOES change")
		})
	}
}

// The status is the money path and must survive the reshape untouched.
func TestEnvelopeVersionBoundary_StatusIsIdenticalAcrossVersions(t *testing.T) {
	app := buildEnvelopeProbeApp(t, true)

	for _, probe := range []string{"/organizations/not-a-uuid", "/probe/not-found", "/probe/boom", "/probe/unprocessable"} {
		t.Run(probe, func(t *testing.T) {
			v1Status, _, _ := driveProbe(t, app, "/v1"+probe)
			v2Status, _, _ := driveProbe(t, app, "/v2"+probe)

			assert.Equal(t, v2Status, v1Status, "the envelope changes shape, never status")
		})
	}
}

// TestEnvelopeVersionBoundary_ReturnedErrors covers the errors that never pass
// through ErrorEnvelope: a handler that RETURNS an error leaves the response empty
// while the middleware chain unwinds, and Fiber's error handler writes the body
// afterwards. Route-not-found, method-not-allowed, the auth 401 and the body-limit
// 413 all reach a client this way.
//
// Without WrapErrorHandler these keep the /v2 envelope on a /v1 route, which is
// exactly the split this change exists to remove — and no assertion elsewhere in
// the suite would notice, because every other case writes its body inside the
// chain.
func TestEnvelopeVersionBoundary_ReturnedErrors(t *testing.T) {
	app := buildEnvelopeProbeApp(t, true)

	cases := []struct {
		name string
		path string
		want int
	}{
		{name: "handler returns an error without writing", path: "/v1/probe/returns-error", want: fiber.StatusNotFound},
		{name: "router finds no route", path: "/v1/no-such-route", want: fiber.StatusNotFound},
		{name: "the bare version path", path: "/v1", want: fiber.StatusNotFound},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			status, body, contentType := driveProbe(t, app, testCase.path)

			assert.Equal(t, testCase.want, status)
			assert.Contains(t, contentType, fiber.MIMEApplicationJSON)
			assert.NotContains(t, contentType, "problem+json")

			var decoded map[string]any
			require.NoError(t, json.Unmarshal([]byte(body), &decoded), "body: %s", body)

			assert.Equal(t, []string{"code", "message", "title"}, sortedBodyKeys(decoded),
				"the v1 envelope carries exactly these three keys")
			assert.NotEmpty(t, decoded["code"])
		})
	}

	t.Run("v2 keeps the problem document", func(t *testing.T) {
		_, body, contentType := driveProbe(t, app, "/v2/probe/returns-error")

		assert.Contains(t, contentType, "application/problem+json",
			"the Fiber path must serve the RFC 9457 media type, not just the shape")
		assert.Contains(t, body, `"type":`)
		assert.Contains(t, body, `"status":`)
		assert.Contains(t, body, `"detail":`)
	})
}

func sortedBodyKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

func buildEnvelopeProbeApp(t *testing.T, withEnvelope bool) *fiber.App {
	t.Helper()

	handler := pkgHTTP.CanonicalFiberErrorHandler
	if withEnvelope {
		handler = ledgerMiddleware.WrapErrorHandler(handler)
	}

	app := fiber.New(fiber.Config{ErrorHandler: handler})

	if withEnvelope {
		app.Use(ledgerMiddleware.ErrorEnvelope())
	}

	for _, prefix := range []string{"/v1", "/v2", ""} {
		group := app.Group(prefix)

		group.Get("/organizations/:organization_id", pkgHTTP.ParseUUIDPathParameters("organization"),
			func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })

		group.Get("/probe/not-found", func(c fiber.Ctx) error {
			return pkgHTTP.WithError(c, pkg.EntityNotFoundError{
				EntityType: "Ledger",
				Code:       "0007",
				Title:      "Entity Not Found",
				Message:    "No entity was found for the given ID.",
			})
		})

		group.Get("/probe/fields", func(c fiber.Ctx) error {
			return pkgHTTP.WithError(c, pkg.ValidationKnownFieldsError{
				EntityType: "Account",
				Code:       "0018",
				Title:      "Missing Fields in Request",
				Message:    "Your request is missing fields.",
				Fields:     pkg.FieldValidations{"name": "name is required"},
			})
		})

		group.Get("/probe/unprocessable", func(c fiber.Ctx) error {
			return pkgHTTP.WithError(c, pkg.UnprocessableOperationError{
				Code:    "0019",
				Title:   "Unprocessable Operation",
				Message: "The operation cannot be completed.",
			})
		})

		group.Get("/probe/boom", func(c fiber.Ctx) error {
			return pkgHTTP.WithError(c, pkg.ValidateInternalError(errors.New("kaboom"), ""))
		})

		// Returns the error instead of writing it: the body is produced by the
		// Fiber error handler after the middleware chain has unwound.
		group.Get("/probe/returns-error", func(c fiber.Ctx) error {
			return fiber.ErrNotFound
		})

		group.Get("/probe/ok", func(c fiber.Ctx) error {
			return c.JSON(fiber.Map{"status": "ok"})
		})
	}

	return app
}

func driveProbe(t *testing.T, app *fiber.App, path string) (int, string, string) {
	t.Helper()

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, string(raw), resp.Header.Get(fiber.HeaderContentType)
}

// TestEnvelopeVersionBoundary_HumaTerminal proves the assumption the whole design
// rests on: a Huma-rendered error body is visible to the outer Fiber middleware.
//
// Huma does not write through fiber's Ctx helpers — humafiber hands it the raw
// RequestCtx (humafiber.go BodyWriter). If that bypassed the response buffer
// ErrorEnvelope reads, every Huma terminal on /v1 would keep serving problem+json
// while the Fiber terminals served the v3 envelope, and nothing else in this suite
// would notice.
func TestEnvelopeVersionBoundary_HumaTerminal(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	app.Use(ledgerMiddleware.ErrorEnvelope())

	api := AssembleHumaContract(app, app, openapi.Config{
		Title: "envelope-probe", Version: "test", Servers: []string{"/"},
	})

	for _, prefix := range []string{"/v1", "/v2"} {
		app.Group(prefix)

		huma.Register(
			huma.NewGroup(api, prefix),
			huma.Operation{
				OperationID: "envelopeProbe" + strings.TrimPrefix(prefix, "/"),
				Method:      http.MethodGet,
				Path:        "/probe/huma",
			},
			func(_ context.Context, _ *struct{}) (*struct{}, error) {
				return nil, pkgHTTP.HumaProblem(pkg.EntityNotFoundError{
					EntityType: "Ledger",
					Code:       "0007",
					Title:      "Entity Not Found",
					Message:    "No entity was found for the given ID.",
				})
			},
		)
	}

	t.Run("v1 huma terminal serves the v3 envelope", func(t *testing.T) {
		status, body, contentType := driveProbe(t, app, "/v1/probe/huma")

		assert.Equal(t, http.StatusNotFound, status)
		assert.JSONEq(t,
			`{"code":"0007","message":"No entity was found for the given ID.","title":"Entity Not Found"}`,
			body)
		assert.Contains(t, contentType, fiber.MIMEApplicationJSON)
		assert.NotContains(t, contentType, "problem+json")
	})

	t.Run("v2 huma terminal keeps problem+json", func(t *testing.T) {
		status, body, contentType := driveProbe(t, app, "/v2/probe/huma")

		assert.Equal(t, http.StatusNotFound, status)
		assert.Contains(t, body, `"type":`)
		assert.Contains(t, body, `"detail":`)
		assert.Contains(t, contentType, "problem+json")
	})
}
