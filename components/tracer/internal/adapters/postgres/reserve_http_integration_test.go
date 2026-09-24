// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi "github.com/LerianStudio/lib-commons/v7/commons/net/http/openapi"
	problem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	httpin "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// Identity is supplied by the fixture at the already-authenticated boundary.
// This exercises Fiber/Huma, strict JSON and real transactional repositories;
// certificate verification and the Ledger binary are outside this test.
func TestIntegrationReserveHTTPPersistsAndReplays(t *testing.T) {
	db := completionDatabase(t)
	admission, policies, request := admissionFixture(t, db)
	admissionPolicy(t, db, policies, model.DecisionAllow)
	limit := admissionLimit(t, db, request, 89601, "100")
	completion, decisions, _ := completionCommand(t, db, true)
	byID, err := command.NewCompleteReserveReservationCommand(decisions, completion, true)
	require.NoError(t, err)
	bounds := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	handler, err := httpin.NewContextReservationHandler(admission, completion, byID, bounds, 65536, 100)
	require.NoError(t, err)
	problem.Install()
	app := fiber.New(fiber.Config{ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler})
	t.Cleanup(func() { require.NoError(t, app.Shutdown()) })
	group := app.Group("/v1", func(c fiber.Ctx) error { c.SetContext(completionContext(c.Context(), "producer")); return c.Next() })
	api := openapi.New(app, group, openapi.Config{Title: "reservation persistence test", Version: "1", Servers: []string{"/v1"}})
	pkgHTTP.InstallSchemaNamer(api)
	httpin.RegisterContextReservationRoutes(api, handler, nil)
	post := func(path string, body any, status int) []byte {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		input := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		input.Header.Set("Content-Type", "application/json")
		response, err := app.Test(input)
		require.NoError(t, err)
		defer response.Body.Close()
		result, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.Equal(t, status, response.StatusCode, string(result))
		return result
	}
	raw := post("/v1/reservations", request, http.StatusCreated)
	first, err := tracercontract.DecodeReserveResultJSON(t.Context(), raw, 65536, 100)
	require.NoError(t, err)
	require.NoError(t, first.ValidateFor(request, 100))
	require.Len(t, first.ReservationIDs, 1)
	require.JSONEq(t, string(raw), string(post("/v1/reservations", request, http.StatusCreated)))
	body := map[string]string{"contractRevision": tracercontract.ReserveContractRevision}
	confirmPath := "/v1/reservations/transaction/" + request.TransactionID.String() + "/confirm"
	post(confirmPath, body, http.StatusOK) // Treat this acknowledgement as lost.
	replay := post(confirmPath, body, http.StatusOK)
	completed, err := tracercontract.DecodeTransactionCompletionJSON(t.Context(), replay, 65536)
	require.NoError(t, err)
	require.NoError(t, completed.Validate())
	require.Zero(t, completed.Flipped)
	post("/v1/reservations/transaction/"+request.TransactionID.String()+"/release", body, http.StatusConflict)
	request.Amount = "11"
	post("/v1/reservations", request, http.StatusConflict)
	current, held := readCounterDecimal(t, db, limit, "acct:"+request.Context.Accounts[0].ID.String(), testutil.FixedTime().Format("2006-01-02"))
	require.Equal(t, "10.125", current.String())
	require.True(t, held.IsZero())
	require.Len(t, completionEvents(t, db, request.TransactionID), 2)
}
