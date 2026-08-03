// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strings"

	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// The fee and billing surface is served at two scopes. The organization-scoped
// surface carries no ledger in its path and reaches a resource on whichever ledger
// of the organization owns it; the ledger-scoped surface names the ledger in the
// path and reaches only what that ledger owns. This file holds what the second
// scope needs and the first does not: the guards that keep the path the sole
// authority on which ledger a request acts within.
//
// Both repositories express "no ledger requested" as a zero value — uuid.Nil for
// packages, the empty string for billing packages — and both read it as
// organization scope. That makes a zero value arriving from a ledger-scoped path a
// silent widening rather than an error, which is why it is refused here rather than
// carried inward.

// ledgerQueryParameter is the query key both list endpoints bind their ledger filter
// from. The fee-package binder lowercases every key before matching, so the
// comparison against it is case-insensitive.
const ledgerQueryParameter = "ledgerId"

// rejectLedgerQueryParameter refuses a ledger filter on a surface whose path already
// names the ledger.
//
// Neither list validates its key SET — the fee-package binder ignores keys it does
// not recognize and the billing binder reads the four it wants out of the map — so
// nothing else in the request path would report the key, and the two readings of it
// (a redundant repetition, or a request for a different ledger) would be answered
// identically. An empty value is refused too: on the organization-scoped surface it
// means "every ledger", which is the one scope a ledger-scoped listing must not be
// able to express.
func rejectLedgerQueryParameter(queries map[string]string) error {
	for key := range queries {
		if strings.EqualFold(key, ledgerQueryParameter) {
			return feeerrors.ValidateBusinessError(feeconstant.ErrLedgerScopedQueryParameter, "", ledgerQueryParameter)
		}
	}

	return nil
}

// requireBodyLedgerMatchesPath refuses a request body that names a different ledger
// than the path.
//
// The ledger field is required on the shared request models: it is the only ledger
// the organization-scoped surface has, and the in-process fee seam reads the same
// types, so a ledger-scoped surface cannot drop it from the wire. It can refuse to
// let it disagree, which is what keeps the path authoritative — the alternative
// leaves a caller's mistake indistinguishable from success on the request that
// decides which package prices a transaction.
//
// An unparseable value yields the same invalid-ledger error the organization-scoped
// surface already returns for it, so only the disagreement is new.
func requireBodyLedgerMatchesPath(bodyLedgerID string, pathLedgerID uuid.UUID) error {
	parsed, err := uuid.Parse(bodyLedgerID)
	if err != nil {
		return feeerrors.ValidateBusinessError(feeconstant.ErrInvalidLedgerID, "")
	}

	return requireLedgerMatchesPath(parsed, pathLedgerID)
}

// requireLedgerMatchesPath is requireBodyLedgerMatchesPath for a body that already
// carries its ledger as a parsed identifier.
func requireLedgerMatchesPath(bodyLedgerID, pathLedgerID uuid.UUID) error {
	if bodyLedgerID != pathLedgerID {
		return feeerrors.ValidateBusinessError(feeconstant.ErrLedgerIDMismatch, "")
	}

	return nil
}

// feeLedgerScopeAttributes describes the scope a fee or billing request is acting
// within. The ledger id is attached only when one was requested, so an
// organization-scoped call is not reported as one targeting the nil ledger — the
// same posture the repositories' scope attributes take.
func feeLedgerScopeAttributes(organizationID, ledgerID uuid.UUID, resourceKey string, resourceID uuid.UUID) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String(resourceKey, resourceID.String()),
	}

	if ledgerID != uuid.Nil {
		attributes = append(attributes, attribute.String("app.request.ledger_id", ledgerID.String()))
	}

	return attributes
}
