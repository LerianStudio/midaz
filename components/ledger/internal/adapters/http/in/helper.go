// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// This file holds the path-parameter helpers every resource's Huma transport
// shares. They live here rather than on one resource's handler file because the
// whole package resolves its path UUIDs through them.
//
// Path params are declared FLAT on each Input struct (not via an embedded shared
// struct): Huma v2's request layer does not populate `path:` tags on anonymous
// embedded structs, so embedding silently leaves org/ledger empty and every core
// call 0065s. Flat fields are the proven shape (mirrors the tracer). The org+ledger
// pair is resolved through the shared parseOrgLedger helper to avoid repetition.

// parseOrgLedger resolves the org+ledger path strings to UUIDs. On the wired path
// the ParseUUIDPathParameters middleware has already validated them, so this never
// errors; the canonical 0065 is returned defensively if it somehow does.
func parseOrgLedger(orgStr, ledgerStr string) (orgID, ledgerID uuid.UUID, err error) {
	orgID, err = parsePathUUID(orgStr, "organization_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	ledgerID, err = parsePathUUID(ledgerStr, "ledger_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return orgID, ledgerID, nil
}

// parsePathUUID mirrors GetUUIDFromLocals' failure envelope (ErrInvalidPathParameter
// / 0065) so a bad path param yields the canonical 400.
func parsePathUUID(value, key string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", key)
	}

	return id, nil
}
