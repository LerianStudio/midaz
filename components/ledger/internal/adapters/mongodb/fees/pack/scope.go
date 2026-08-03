// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

// packageScopeFilter builds the by-ID lookup filter every single-package read and
// write shares, so the scope the money path depends on is spelled once.
//
// uuid.Nil is the organization-scope sentinel here: ledger_id is left out and the
// package matches on whichever ledger owns it. It is the same sentinel the billing
// aggregate names AnyLedger — that one needs a name because its ledger is a string,
// whose zero value reads as an accident rather than as a request. Any other ledgerID
// restricts the match to that ledger, so a package owned by a different ledger of the
// same organization does not match at all — the caller sees the same no-match a
// nonexistent id produces, and learns nothing about the other ledger.
//
// Soft-deleted documents are excluded under both scopes.
func packageScopeFilter(id, organizationID, ledgerID uuid.UUID) bson.M {
	filter := bson.M{
		"_id":             id,
		"organization_id": organizationID,
		"deleted_at":      bson.M{"$eq": nil},
	}

	if ledgerID != uuid.Nil {
		filter["ledger_id"] = ledgerID
	}

	return filter
}

// packageScopeAttributes describes the requested scope on a span. The ledger id is
// only attached when one was requested, so an organization-scoped call is not
// reported as one targeting the nil ledger.
func packageScopeAttributes(id, organizationID, ledgerID uuid.UUID) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
		attribute.Bool("app.request.has_ledger_id", ledgerID != uuid.Nil),
	}

	if ledgerID != uuid.Nil {
		attributes = append(attributes, attribute.String("app.request.ledger_id", ledgerID.String()))
	}

	return attributes
}
