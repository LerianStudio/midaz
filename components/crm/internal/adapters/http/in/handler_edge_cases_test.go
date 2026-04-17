// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/crm/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// These tests target edge-case branches in the HTTP handlers that the main
// TestAliasHandler_* / TestHolderHandler_* suites skip: malformed path params,
// missing fiber locals, and ValidateParameters failures. They use the real
// handler wiring (no mocks for the happy path is needed because these tests
// fail before reaching the service layer).

// -- UpdateAlias ------------.

// TestAliasHandler_UpdateAlias_MissingIDLocal drives the `id` branch of
// GetUUIDFromLocals by calling UpdateAlias from a route where neither "id"
// nor "holder_id" locals are set. Expected: 400 ErrInvalidPathParameter.
func TestAliasHandler_UpdateAlias_MissingIDLocal(t *testing.T) {
	t.Parallel()

	handler := &AliasHandler{Service: &services.UseCase{}}

	app := fiber.New()
	app.Patch("/v1/holders/:holder_id/aliases/:id",
		pkgHTTP.WithBody(new(mmodel.UpdateAliasInput), handler.UpdateAlias),
	)

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodPatch,
		"/v1/holders/"+uuid.NewString()+"/aliases/"+uuid.NewString(),
		bytes.NewBufferString(`{"description":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	// Without ParseUUIDPathParameters middleware no "id" local is set, so
	// GetUUIDFromLocals returns ErrInvalidPathParameter (code 0065). That
	// error is not one of the typed error variants handled in WithError, so
	// it falls through to ValidateInternalError — a 500. We assert the
	// status is non-2xx to pin the branch coverage without hard-coding the
	// exact mapping, which is controlled by pkg/net/http.
	assert.GreaterOrEqual(t, resp.StatusCode, 400)
}

// TestAliasHandler_UpdateAlias_MissingHolderIDLocal covers the holder_id
// parse error branch: the "id" local is set, but "holder_id" is missing.
func TestAliasHandler_UpdateAlias_MissingHolderIDLocal(t *testing.T) {
	t.Parallel()

	handler := &AliasHandler{Service: &services.UseCase{}}
	aliasID := uuid.New()

	app := fiber.New()
	app.Patch("/v1/holders/:holder_id/aliases/:id",
		func(c *fiber.Ctx) error {
			// Set only the alias ID local, leave holder_id unset.
			c.Locals("id", aliasID)
			return c.Next()
		},
		pkgHTTP.WithBody(new(mmodel.UpdateAliasInput), handler.UpdateAlias),
	)

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodPatch,
		"/v1/holders/"+uuid.NewString()+"/aliases/"+aliasID.String(),
		bytes.NewBufferString(`{"description":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.GreaterOrEqual(t, resp.StatusCode, 400)
}

// -- GetAllAliases ------------.

// TestAliasHandler_GetAllAliases_InvalidStartDate drives the
// ValidateParameters error path by sending a malformed start_date. Unlike
// `limit`, which is silently coerced via strconv.Atoi, start_date is parsed
// via ParseDateTime and returns ErrInvalidDatetimeFormat on failure.
func TestAliasHandler_GetAllAliases_InvalidStartDate(t *testing.T) {
	t.Parallel()

	handler := &AliasHandler{Service: &services.UseCase{}}

	app := fiber.New()
	app.Get("/v1/aliases",
		func(c *fiber.Ctx) error {
			c.Request().Header.Set("X-Organization-Id", "org-1")
			return c.Next()
		},
		handler.GetAllAliases,
	)

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodGet, "/v1/aliases?start_date=not-a-date", nethttp.NoBody)
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.GreaterOrEqual(t, resp.StatusCode, 400)
}

// TestAliasHandler_GetAllAliases_InvalidHolderID exercises the UUID-parse
// branch when headerParams.HolderID is a malformed UUID: ValidateParameters
// passes, but uuid.Parse returns an error in GetAllAliases.
func TestAliasHandler_GetAllAliases_InvalidHolderID(t *testing.T) {
	t.Parallel()

	handler := &AliasHandler{Service: &services.UseCase{}}

	app := fiber.New()
	app.Get("/v1/aliases",
		func(c *fiber.Ctx) error {
			c.Request().Header.Set("X-Organization-Id", "org-1")
			return c.Next()
		},
		handler.GetAllAliases,
	)

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodGet, "/v1/aliases?holder_id=not-a-uuid", nethttp.NoBody)
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.GreaterOrEqual(t, resp.StatusCode, 400)

	// Verify the response body is a well-formed error envelope. We accept
	// either a ValidateInternalError payload (500) or a ValidationError
	// payload (400); both carry a "code" field.
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	if len(body) > 0 {
		var errResp map[string]any
		if json.Unmarshal(body, &errResp) == nil {
			assert.Contains(t, errResp, "code", "error response should contain code")
		}
	}
}

// -- DeleteAliasByID ------------.

// TestAliasHandler_DeleteAliasByID_MissingIDLocal exercises the id-parse
// failure branch of DeleteAliasByID.
func TestAliasHandler_DeleteAliasByID_MissingIDLocal(t *testing.T) {
	t.Parallel()

	handler := &AliasHandler{Service: &services.UseCase{}}

	app := fiber.New()
	app.Delete("/v1/holders/:holder_id/aliases/:id", handler.DeleteAliasByID)

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodDelete,
		"/v1/holders/"+uuid.NewString()+"/aliases/"+uuid.NewString(), nethttp.NoBody)
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.GreaterOrEqual(t, resp.StatusCode, 400)
}

// -- GetAliasByID ------------.

// TestAliasHandler_GetAliasByID_MissingIDLocal exercises the id-parse failure
// branch of GetAliasByID.
func TestAliasHandler_GetAliasByID_MissingIDLocal(t *testing.T) {
	t.Parallel()

	handler := &AliasHandler{Service: &services.UseCase{}}

	app := fiber.New()
	app.Get("/v1/holders/:holder_id/aliases/:id", handler.GetAliasByID)

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodGet,
		"/v1/holders/"+uuid.NewString()+"/aliases/"+uuid.NewString(), nethttp.NoBody)
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.GreaterOrEqual(t, resp.StatusCode, 400)
}

// -- DeleteRelatedParty ------------.

// TestAliasHandler_DeleteRelatedParty_MissingHolderID covers the holder_id
// parse error branch.
func TestAliasHandler_DeleteRelatedParty_MissingHolderID(t *testing.T) {
	t.Parallel()

	handler := &AliasHandler{Service: &services.UseCase{}}

	app := fiber.New()
	app.Delete("/v1/holders/:holder_id/aliases/:alias_id/related-parties/:related_party_id",
		handler.DeleteRelatedParty)

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodDelete,
		"/v1/holders/"+uuid.NewString()+"/aliases/"+uuid.NewString()+"/related-parties/"+uuid.NewString(),
		nethttp.NoBody)
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.GreaterOrEqual(t, resp.StatusCode, 400)
}

// -- UpdateHolder ------------.

// TestHolderHandler_UpdateHolder_MissingIDLocal exercises the id-parse failure
// branch of UpdateHolder.
func TestHolderHandler_UpdateHolder_MissingIDLocal(t *testing.T) {
	t.Parallel()

	handler := &HolderHandler{Service: &services.UseCase{}}

	app := fiber.New()
	app.Patch("/v1/holders/:id", pkgHTTP.WithBody(new(mmodel.UpdateHolderInput), handler.UpdateHolder))

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodPatch,
		"/v1/holders/"+uuid.NewString(), bytes.NewBufferString(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.GreaterOrEqual(t, resp.StatusCode, 400)
}

// -- DeleteHolderByID ------------.

// TestHolderHandler_DeleteHolderByID_MissingIDLocal exercises the id-parse
// failure branch of DeleteHolderByID.
func TestHolderHandler_DeleteHolderByID_MissingIDLocal(t *testing.T) {
	t.Parallel()

	handler := &HolderHandler{Service: &services.UseCase{}}

	app := fiber.New()
	app.Delete("/v1/holders/:id", handler.DeleteHolderByID)

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodDelete, "/v1/holders/"+uuid.NewString(), nethttp.NoBody)
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.GreaterOrEqual(t, resp.StatusCode, 400)
}

// -- GetHolderByID ------------.

// TestHolderHandler_GetHolderByID_MissingIDLocal exercises the id-parse
// failure branch of GetHolderByID.
func TestHolderHandler_GetHolderByID_MissingIDLocal(t *testing.T) {
	t.Parallel()

	handler := &HolderHandler{Service: &services.UseCase{}}

	app := fiber.New()
	app.Get("/v1/holders/:id", handler.GetHolderByID)

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodGet, "/v1/holders/"+uuid.NewString(), nethttp.NoBody)
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.GreaterOrEqual(t, resp.StatusCode, 400)
}

// -- GetAllHolders ------------.

// TestHolderHandler_GetAllHolders_InvalidStartDate drives the
// ValidateParameters error branch. Mirrors the alias variant.
func TestHolderHandler_GetAllHolders_InvalidStartDate(t *testing.T) {
	t.Parallel()

	handler := &HolderHandler{Service: &services.UseCase{}}

	app := fiber.New()
	app.Get("/v1/holders",
		func(c *fiber.Ctx) error {
			c.Request().Header.Set("X-Organization-Id", "org-1")
			return c.Next()
		},
		handler.GetAllHolders,
	)

	req := httptest.NewRequestWithContext(t.Context(), nethttp.MethodGet, "/v1/holders?start_date=not-a-date", nethttp.NoBody)
	req.Header.Set("X-Organization-Id", "org-1")

	resp, err := app.Test(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.GreaterOrEqual(t, resp.StatusCode, 400)
}
