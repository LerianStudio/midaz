// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"flag"
	"os"
	"testing"

	authMiddleware "github.com/LerianStudio/lib-auth/v2/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in/middleware"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// update, when set, rewrites the committed golden spec instead of asserting
// against it: `go test ./... -run TestOpenAPISpecDump -update -buildvcs=false`.
var update = flag.Bool("update", false, "rewrite the committed OpenAPI 3.1 golden spec")

// humaSpecPath is the committed native Huma OAS 3.1 dump. It is DELIBERATELY a
// distinct filename from api/openapi.yaml — that file is the swaggo/openapi-
// generator OAS 3.0.1 artifact the active docs pipeline still consumes. This dump
// is additive: it writes the Huma snapshot beside the swaggo one so neither
// clobbers the other until the pipeline is rewired onto Huma in a later phase.
const humaSpecPath = "../../../../api/openapi.huma.yaml"

// buildTracerHumaAPI mirrors the SPEC surface of NewRoutes: it runs the same
// problem.Install → openapi.New → InstallSchemaNamer → DeclareBearerAuth +
// ApiKeyAuth setup, then mounts every Huma op via the shared registerTracerHumaRoutes
// seam (task-2). Registration reads handler types only — it never invokes them — so
// zero-value handlers are safe. Reservation is wired non-nil (its 5 ops are in the
// served spec, per routes_openapi_security_test.go's 28-op table); its tenant
// middleware is a no-op passthrough since registration doesn't execute it. The
// returned huma.API's OpenAPI() is the same object openapi.ServeSpec serializes at
// runtime — this just reads it offline, no server or DB.
func buildTracerHumaAPI() huma.API {
	f := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ErrorHandler:          pkgHTTP.CanonicalFiberErrorHandler,
	})

	// problem.Install must run before any huma.Register (runtime + spec-gen).
	libProblem.Install()

	api := f.Group("/v1")

	humaAPI := openapi.New(f, api, openapi.Config{
		Title: "Midaz Tracer API",
		// Hardcoded (not os.Getenv("VERSION")) so the golden dump is hermetic and
		// env-independent, matching the ledger dump (contract_spec_routes_test.go).
		// info.version carries no contract value in an offline golden; a CI job that
		// exported VERSION would otherwise drift this golden and fail an unrelated PR.
		Version: "4.0.0",
		Servers: []string{"/v1"},
	})

	// Rename the shared problem.Detail error body to "Error" before any
	// huma.Register (the registry namer is captured on first registration).
	pkgHTTP.InstallSchemaNamer(humaAPI)

	// Declare the two security schemes the per-op Security metadata references, so
	// the dumped spec resolves them instead of dangling — same as NewRoutes.
	openapi.DeclareBearerAuth(humaAPI)

	components := humaAPI.OpenAPI().Components
	if components.SecuritySchemes == nil {
		components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}

	components.SecuritySchemes["ApiKeyAuth"] = &huma.SecurityScheme{
		Type:        "apiKey",
		In:          "header",
		Name:        "X-API-Key",
		Description: "Static API key presented in the X-API-Key header.",
	}

	authLogger := libLog.NewNop()
	authClient := authMiddleware.NewAuthClient("", false, &authLogger)
	guard := middleware.NewAuthGuard(middleware.AuthGuardConfig{AppName: "tracer"}, authClient)

	registerTracerHumaRoutes(api, humaAPI, tracerHumaHandlers{
		Guard:                 guard,
		Rule:                  &Handler{},
		Limit:                 &LimitHandler{},
		TransactionValidation: &TransactionValidationHandler{},
		Validation:            &ValidationHandler{},
		Reservation:           &ReservationHandler{},
		ResTenantMW:           func(c *fiber.Ctx) error { return c.Next() },
		AuditEvent:            &AuditEventHandler{},
	})

	return humaAPI
}

// TestOpenAPISpecDump is the golden gate for the tracer's native Huma OAS 3.1
// spec. It snapshots the shared huma.API AFTER every huma.Register (via the same
// registerTracerHumaRoutes seam NewRoutes uses) and serializes it offline — no
// server, DB, or Docker. huma.OpenAPI serialization is deterministic (top-level
// fields emit in a fixed order; every nested map — Paths, Components.Schemas —
// goes through encoding/json, which sorts map keys), so two runs without -update
// produce identical bytes and the diff is stable.
//
// With -update it rewrites the golden file; without it, it fails on any drift.
//
// NOT parallel: buildTracerHumaAPI mutates process-global huma state
// (problem.Install + the shared schema registry). Same reason the huma unit tests
// run sequentially — see buildHumaRuleApp.
func TestOpenAPISpecDump(t *testing.T) {
	got, err := buildTracerHumaAPI().OpenAPI().YAML()
	require.NoError(t, err, "serialize huma OpenAPI to YAML")

	if *update {
		require.NoError(t, os.WriteFile(humaSpecPath, got, 0o644),
			"write golden spec %s", humaSpecPath)
		t.Logf("wrote golden spec %s (%d bytes)", humaSpecPath, len(got))

		return
	}

	want, err := os.ReadFile(humaSpecPath)
	require.NoErrorf(t, err, "read golden spec %s (run with -update to generate)", humaSpecPath)
	require.Equalf(t, string(want), string(got),
		"native Huma OAS 3.1 spec drifted from %s; run `go test -run TestOpenAPISpecDump -update -buildvcs=false` to regenerate",
		humaSpecPath)
}
