// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package billing_package

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

// AnyLedger is the ledger argument that puts a query under organization scope: no
// ledger clause is added, so it matches a billing package on whichever ledger of
// the organization owns it. FindAll reads it the same way and lists every ledger.
// It is the string-typed counterpart of uuid.Nil in the fee-package aggregate, and
// it carries a name because an empty string otherwise reads as an omission.
//
// A surface whose path names a ledger must never pass it. It widens the query to
// the whole organization, which on such a path would answer with packages that
// ledger does not own.
const AnyLedger = ""

// billingPackageScopeFilter builds the by-ID lookup filter every single-package
// read and write shares, so the scope the money path depends on is spelled once.
//
// A ledgerID of AnyLedger means organization scope. Any other ledgerID restricts
// the match to that ledger, so a package owned by a different ledger of the same
// organization does not match at all — the caller sees the same no-match a
// nonexistent id produces, and learns nothing about the other ledger.
//
// Soft-deleted documents are excluded under both scopes.
func billingPackageScopeFilter(id, organizationID, ledgerID string) bson.M {
	filter := bson.M{
		"_id":             id,
		"organization_id": organizationID,
		"deleted_at":      bson.M{"$eq": nil},
	}

	if ledgerID != AnyLedger {
		filter["ledger_id"] = ledgerID
	}

	return filter
}

// billingPackageScopeAttributes describes the requested scope on a span. The
// ledger id is only attached when one was requested, so an organization-scoped
// call is not reported as one targeting an empty ledger.
func billingPackageScopeAttributes(id, organizationID, ledgerID string) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		attribute.String("app.request.organization_id", organizationID),
		attribute.String("app.request.billing_package_id", id),
		attribute.Bool("app.request.has_ledger_id", ledgerID != AnyLedger),
	}

	if ledgerID != AnyLedger {
		attributes = append(attributes, attribute.String("app.request.ledger_id", ledgerID))
	}

	return attributes
}
