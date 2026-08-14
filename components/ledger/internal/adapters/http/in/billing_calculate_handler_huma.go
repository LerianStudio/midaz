// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// This file holds the shared response envelope for the billing-calculate (compute) op.
// The ledger-scoped handler that constructs it lives in fees_v2_handler.go; the auth
// ("plugin-fees","billing-calculate","post") + tenant +
// ParseUUIDPathParameters("billing-calculate") middleware chain is attached on the /v2
// Fiber group BEFORE the Huma terminal (see fees_v2_register.go).
//
// 200 is intentional: this is a compute/RPC-style endpoint that persists nothing.
// Unlike the fee-estimate op (whose response embeds the transaction tree and forces a
// raw-[]byte escape hatch), BillingCalculateResponse is a flat Results+Summary struct
// with no time.Time-alias schema-gen landmine, so it serializes as a normal typed Body.

// CalculateBillingOutputHuma carries the calculation envelope at 200.
type CalculateBillingOutputHuma struct {
	Status int
	Body   *model.BillingCalculateResponse
}
