// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// =============================================================================
// ACCOUNT-BLOCK EXCEPTION SURFACES — transport verdicts
// =============================================================================
// The surface rule (decision D2) is that direct, commit and revert accept a
// presented identifier and the HOLD does not. The decode-level half of that rule
// is covered in transaction_lifecycle_body_v2_test.go; what is pinned here is the
// verdict a CLIENT actually receives, because "rejected" is only useful if it
// arrives as a 4xx naming the field rather than as a 500 or a silent drop.

// v2HoldBodyWithException is a fully valid hold body — every structural field the
// translate step needs is present — carrying the one field the hold refuses. So
// the only thing that can reject it is the surface rule.
const v2HoldBodyWithException = `{"description":"v2 hold","asset":"BRL","amount":"100",` +
	`"accountBlockExceptionId":"018f2c1e-6a3b-7c4d-8e5f-0a1b2c3d4e5f",` +
	`"debits":[{"alias":"@src",` + v2ScopeJSON + `,"amount":"100"}],` +
	`"credits":[{"alias":"@dst",` + v2ScopeJSON + `,"amount":"100"}]}`

// TestD2_HoldRejectsAPresentedExceptionWith400 pins the client-visible half of
// decision D2.
//
// A two-phase transaction would need two authorizations — one for the hold and
// one for the commit — so the hold takes none, and a caller who presents one on
// it must be TOLD. Dropping the field silently would be the worst outcome: the
// pending would look authorized, and the commit would fail later with an error
// naming no field the caller ever sent.
//
// The comparison case is the same body WITHOUT the field: it must clear the
// transport boundary, which is what proves the 400 came from the surface rule and
// not from something else in the body.
func TestD2_HoldRejectsAPresentedExceptionWith400(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2ActionApp(t, "hold", (&TransactionHandler{}).CreateTransactionHoldV2)

	resp := postActionV2(t, app, "hold", v2HoldBodyWithException)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"a hold presenting an account-block exception is a client error, not a server one")
	assert.Contains(t, string(body), cn.ErrAccountBlockExceptionNotSupported.Error(),
		"the problem document must carry the surface-rule code so the caller can act on it")
	assert.Contains(t, string(body), "accountBlockExceptionId",
		"the rejection must name the offending field")
}

// TestD2_DirectAcceptsAPresentedException is the positive half of the same rule on
// the same transport: the identical body on the DIRECT action clears decode and
// translate and reaches the funnel, so the 400 above is the surface rule biting
// and nothing else.
//
// A bare handler leaves the funnel's first repository call unwired, so reaching it
// surfaces as a recovered 500 — the established signal in this file's neighbours
// that a request progressed past the transport boundary.
func TestD2_DirectAcceptsAPresentedException(t *testing.T) {
	// NOT parallel: process-global huma state.
	app := buildHumaV2DirectApp(t, &TransactionHandler{})

	resp := postDirectV2(t, app, v2HoldBodyWithException)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
		"the direct action must carry a presented exception past the transport boundary")
}
