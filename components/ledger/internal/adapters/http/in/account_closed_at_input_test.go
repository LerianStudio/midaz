// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// --- the guard itself ---------------------------------------------------------

// A body naming the closing instant is refused whatever the value is. The null,
// false and zero cases are the reason the guard exists at all: the shared
// unknown-field detection cannot see them.
func TestRejectAccountClosedAtInput_RefusesEveryValue(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"timestamp":   `"2026-03-04T05:06:07Z"`,
		"null":        `null`,
		"false":       `false`,
		"true":        `true`,
		"zero":        `0`,
		"empty":       `""`,
		"object":      `{"at":"2026-03-04T05:06:07Z"}`,
		"array":       `[]`,
		"nested null": `{"at":null}`,
	}

	for _, key := range accountClosedAtInputKeys {
		for name, value := range values {
			key, name, value := key, name, value

			t.Run(key+"_"+name+"_alone", func(t *testing.T) {
				t.Parallel()

				err := rejectAccountClosedAtInput([]byte(`{"` + key + `":` + value + `}`))
				requireUnexpectedField(t, err, key)
			})

			t.Run(key+"_"+name+"_beside_an_allowed_field", func(t *testing.T) {
				t.Parallel()

				err := rejectAccountClosedAtInput([]byte(`{"name":"Treasury","` + key + `":` + value + `}`))
				requireUnexpectedField(t, err, key)
			})
		}
	}
}

// Both spellings in one body are both reported, so the caller learns to remove
// each of them rather than discovering the second on a retry.
func TestRejectAccountClosedAtInput_ReportsBothSpellings(t *testing.T) {
	t.Parallel()

	err := rejectAccountClosedAtInput([]byte(`{"closedAt":null,"closed_at":0}`))

	var unknown pkg.ValidationUnknownFieldsError
	require.ErrorAs(t, err, &unknown)
	assert.Contains(t, unknown.Fields, "closedAt")
	assert.Contains(t, unknown.Fields, "closed_at")
}

// The guard is localized: it refuses only its own two keys and leaves every
// other body alone, so the general tolerance of the account APIs is unchanged
// and malformed JSON keeps its own contract.
func TestRejectAccountClosedAtInput_LetsEveryOtherBodyThrough(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "allowed fields only", body: `{"name":"Treasury","assetCode":"USD","type":"deposit"}`},
		{name: "empty object", body: `{}`},
		{name: "another null field", body: `{"segmentId":null}`},
		{name: "a key that merely contains the name", body: `{"metadata":{"closedAt":"2026-03-04T05:06:07Z"}}`},
		{name: "a similarly named key", body: `{"closedAtBy":"someone","notClosedAt":true}`},
		{name: "malformed json", body: `{"name":`},
		{name: "empty body", body: ``},
		{name: "json array", body: `[{"closedAt":null}]`},
		{name: "json scalar", body: `"closedAt"`},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.NoError(t, rejectAccountClosedAtInput([]byte(tc.body)))
		})
	}
}

func requireUnexpectedField(t *testing.T, err error, key string) {
	t.Helper()

	require.Error(t, err, "the body names %q and must be refused", key)

	var unknown pkg.ValidationUnknownFieldsError
	require.ErrorAs(t, err, &unknown, "the refusal must reuse the unexpected-field error")
	assert.Contains(t, unknown.Fields, key, "the refusal must name the offending field")
}

// --- the wiring ---------------------------------------------------------------

// closedAtRejectionHandler is an account handler whose repositories are mocks
// with NO expectations: any call through to the service fails the test, which is
// what proves the rejection happens before the mutation.
func closedAtRejectionHandler(t *testing.T) *AccountHandler {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	return &AccountHandler{Command: &command.UseCase{
		AccountRepo:            account.NewMockRepository(ctrl),
		AssetRepo:              asset.NewMockRepository(ctrl),
		OnboardingMetadataRepo: mongodb.NewMockRepository(ctrl),
		BalanceRepo:            balance.NewMockRepository(ctrl),
	}}
}

// buildHumaAccountV2App mirrors buildHumaAccountApp on the /v2 mount, so the
// same rejection can be probed on both contracts.
//
// MUST-NOT-PARALLELIZE: libProblem.Install swaps process-global huma state.
func buildHumaAccountV2App(t *testing.T, handler *AccountHandler) *fiber.App {
	t.Helper()

	f := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})

	libProblem.Install()

	f.Use(ledgerMiddleware.ErrorEnvelope())

	apiV2 := f.Group("/v2")

	hAPI := openapi.New(f, apiV2, openapi.Config{Title: "ledger-test", Version: "test", Servers: []string{"/v2"}})

	parse := pkgHTTP.ParseUUIDPathParameters("account")
	base := "/organizations/:organization_id/ledgers/:ledger_id/accounts"
	apiV2.Post(base, parse)
	apiV2.Patch(base+"/:id", parse)

	RegisterAccountV2Routes(hAPI, handler, v2OpSuffix)

	return f
}

// TestAccountRoutes_RejectClosedAtInput drives the rejection through the real
// transport on both contracts, for create and for PATCH. The instant is not an
// input on either, so each request is a 400 and the use case is never reached.
func TestAccountRoutes_RejectClosedAtInput(t *testing.T) {
	// NOT parallel: process-global huma state.
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	base := "/organizations/" + orgID.String() + "/ledgers/" + ledgerID.String() + "/accounts"

	// Each offending key rides a body that is otherwise VALID for its operation,
	// so the 400 can only come from the guard: without it the request would reach
	// the use case and the expectation-free mocks would fail the test.
	values := map[string]string{
		"timestamp":             `"closedAt":"2026-03-04T05:06:07Z"`,
		"null":                  `"closedAt":null`,
		"false":                 `"closedAt":false`,
		"zero":                  `"closedAt":0`,
		"column spelling null":  `"closed_at":null`,
		"column spelling stamp": `"closed_at":"2026-03-04T05:06:07Z"`,
	}

	routes := []struct {
		name     string
		version  string
		method   string
		path     string
		validFor string
	}{
		{name: "v1 create", version: "v1", method: http.MethodPost, path: base, validFor: `"name":"Treasury","assetCode":"USD","type":"deposit"`},
		{name: "v1 update", version: "v1", method: http.MethodPatch, path: base + "/" + accountID.String(), validFor: `"name":"Treasury"`},
		{name: "v2 create", version: "v2", method: http.MethodPost, path: base, validFor: `"name":"Treasury","assetCode":"USD","type":"deposit"`},
		{name: "v2 update", version: "v2", method: http.MethodPatch, path: base + "/" + accountID.String(), validFor: `"name":"Treasury"`},
	}

	for _, route := range routes {
		for name, value := range values {
			body := `{` + route.validFor + `,` + value + `}`

			t.Run(route.name+"_"+name, func(t *testing.T) {
				handler := closedAtRejectionHandler(t)

				var app *fiber.App
				if route.version == "v1" {
					app = buildHumaAccountApp(t, handler, true)
				} else {
					app = buildHumaAccountV2App(t, handler)
				}

				req := httptest.NewRequest(route.method, "/"+route.version+route.path, bytes.NewReader([]byte(body)))
				req.Header.Set("Content-Type", "application/json")

				resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
				require.NoError(t, err)

				defer func() { _ = resp.Body.Close() }()

				respBody, _ := io.ReadAll(resp.Body)

				require.Equal(t, http.StatusBadRequest, resp.StatusCode, "body: %s", string(respBody))

				var got map[string]any
				require.NoError(t, json.Unmarshal(respBody, &got), "body: %s", string(respBody))
				assert.NotEmpty(t, got["code"], "the refusal must carry its canonical code")
			})
		}
	}
}
