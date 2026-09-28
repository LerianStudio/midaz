// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Fixed scope identifiers for the decode-boundary bodies below; no request reaches a
// repository, so any well-formed UUID pair does.
const (
	skipScopeOrgID    = "11111111-1111-1111-1111-111111111111"
	skipScopeLedgerID = "22222222-2222-2222-2222-222222222222"
)

// The per-call control opt-outs (skip.fees, skip.tracer) exist only on the /v2 create
// input, because the controls they opt out of — the fee engine and the tracer reservation
// — exist only on the /v2 contract. A /v1 body naming `skip` is therefore an unknown
// field and is rejected by the decoder as a clean 400, the same answer any other unknown
// field gets. The consequence is deliberate: `fees_skipped` / `tracer_skipped` can never
// be true on a row created through /v1.

// v1SkipBodies is one minimally-shaped body per /v1 create route that names `skip`. Only
// the decode boundary is under test, so the bodies need not be economically valid.
var v1SkipBodies = map[string]string{
	"/v1/organizations/%[1]s/ledgers/%[2]s/transactions/json": `{
		"skip": {"fees": true},
		"send": {"asset": "BRL", "value": "100",
			"source": {"from": [{"accountAlias": "@payer", "amount": {"asset": "BRL", "value": "100"}}]},
			"distribute": {"to": [{"accountAlias": "@payee", "amount": {"asset": "BRL", "value": "100"}}]}}
	}`,
	"/v1/organizations/%[1]s/ledgers/%[2]s/transactions/inflow": `{
		"skip": {"fees": true},
		"send": {"asset": "BRL", "value": "100",
			"distribute": {"to": [{"accountAlias": "@payee", "amount": {"asset": "BRL", "value": "100"}}]}}
	}`,
	"/v1/organizations/%[1]s/ledgers/%[2]s/transactions/outflow": `{
		"skip": {"fees": true},
		"send": {"asset": "BRL", "value": "100",
			"source": {"from": [{"accountAlias": "@payer", "amount": {"asset": "BRL", "value": "100"}}]}}
	}`,
}

// TestV1CreateRejectsSkipAsUnknownField proves every /v1 create route answers a body
// naming `skip` with the unknown-field 400, so the field cannot be smuggled onto a
// contract that has no control to skip.
func TestV1CreateRejectsSkipAsUnknownField(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaTransactionApp(t, bareTransactionHandler(), true)

	for pathTemplate, body := range v1SkipBodies {
		path := strings.NewReplacer("%[1]s", skipScopeOrgID, "%[2]s", skipScopeLedgerID).Replace(pathTemplate)

		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
		require.NoError(t, err)

		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())

		assert.Equalf(t, http.StatusBadRequest, resp.StatusCode,
			"%s: a /v1 body naming skip must be rejected as an unknown field; body: %s", path, string(raw))

		var problem map[string]any
		require.NoError(t, json.Unmarshal(raw, &problem))
		assert.Equalf(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), problem["code"],
			"%s: the rejection must be the unknown-field code", path)
	}
}

// TestV2CreateAcceptsSkip is the counterpart: the same block on the /v2 direct body is a
// KNOWN field, so the decoder lets it through and the request is answered on its merits
// rather than rejected for naming it.
func TestV2CreateAcceptsSkip(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	body := `{
		"asset": "BRL",
		"amount": "100",
		"skip": {"fees": true, "tracer": true},
		"debits": [{"alias": "@payer", "organizationId": "` + skipScopeOrgID + `", "ledgerId": "` + skipScopeLedgerID + `", "amount": "100"}],
		"credits": [{"alias": "@payee", "organizationId": "` + skipScopeOrgID + `", "ledgerId": "` + skipScopeLedgerID + `", "amount": "100"}]
	}`

	resp := postDirectV2(t, app, body)

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	if resp.StatusCode == http.StatusBadRequest {
		var problem map[string]any
		require.NoError(t, json.Unmarshal(raw, &problem))
		assert.NotEqual(t, constant.ErrUnexpectedFieldsInTheRequest.Error(), problem["code"],
			"skip is a KNOWN field on the /v2 create input; body: %s", string(raw))
	}
}
