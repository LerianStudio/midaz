// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package skip holds the two-key resolver shared by every per-call control
// skip (tracer, fees, holder). It lives in a neutral, inner-importable
// location so both adapters/http/in and services/command can use it without
// an import cycle or an inward-layer violation.
package skip

import (
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ResolveSkipFor applies the two-key rule for a single control: a skip is
// honored only when both the caller requested it and the ledger opt-in allows
// it. A skip requested without the opt-in is a 422 business error; the control
// label is interpolated into the message so the rejection is actionable. An
// unrequested skip is a no-op regardless of the opt-in.
func ResolveSkipFor(control string, requested, allowed bool) (bool, error) {
	switch {
	case requested && !allowed:
		return false, pkg.ValidateBusinessError(constant.ErrSkipNotPermitted, constant.EntityLedger, control)
	case requested && allowed:
		return true, nil
	default:
		return false, nil
	}
}
