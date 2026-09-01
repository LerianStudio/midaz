// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
)

// jsonRequestSchema returns the application/json request-body schema of the op whose
// OperationID matches operationID, or nil if the document carries no such JSON body op.
func jsonRequestSchema(doc *huma.OpenAPI, operationID string) *huma.Schema {
	for _, item := range doc.Paths {
		for _, op := range operationsOf(item) {
			if op.OperationID != operationID || op.RequestBody == nil {
				continue
			}

			if media, ok := op.RequestBody.Content["application/json"]; ok && media != nil {
				return media.Schema
			}
		}
	}

	return nil
}

// isBinaryBodySchema reports whether s is Huma's opaque RawBody schema
// (type: string, format: binary) rather than a typed object / $ref. This is the
// unstructured shape a RawBody []byte + SkipValidateBody op publishes before the
// doc-only typed-body override runs.
func isBinaryBodySchema(s *huma.Schema) bool {
	return s != nil && s.Ref == "" && s.Type == "string" && s.Format == "binary"
}

// jsonBodyOperationIDs collects, over the whole document, the OperationID of every op
// that declares an application/json request body — the exact set the typed-body
// override must cover. Deriving the set from the served surface (not a hardcoded
// number) keeps the completeness assertion honest as ops are added or removed.
func jsonBodyOperationIDs(doc *huma.OpenAPI) []string {
	var ids []string

	for _, item := range doc.Paths {
		for _, op := range operationsOf(item) {
			if op.RequestBody == nil {
				continue
			}

			if media, ok := op.RequestBody.Content["application/json"]; ok && media != nil {
				ids = append(ids, op.OperationID)
			}
		}
	}

	return ids
}

// TestTypedRequestBodyRepresentativeSpread pins that a representative spread of body
// ops on BOTH the /v1 and /v2 surfaces publishes a TYPED request-body schema (a $ref
// to the concrete input component, or at minimum a structured object) rather than the
// opaque type:string/format:binary RawBody schema. It spans an onboarding create on
// both version twins, an onboarding update, a money-path create, and the ledger-scoped
// (v2-only) fees create, so a regression that reverts any one family surfaces here.
func TestTypedRequestBodyRepresentativeSpread(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()

	// operationID -> the EXACT component the $ref must resolve to. Asserting the target
	// name (not just non-empty) catches a wrong-T at a call site: a body op wired to the
	// wrong concrete type would still publish a $ref, but to the wrong component.
	refOps := map[string]string{
		"createOrganization":    "CreateOrganizationInput",
		"createOrganizationV2":  "CreateOrganizationInput",
		"createAccount":         "CreateAccountInput",
		"updateLedger":          "UpdateLedgerInput",
		"createTransactionJSON": "CreateTransactionInput",
		"createPackageV2":       "FeeCreatePackageInput",
	}

	for id, wantComponent := range refOps {
		schema := jsonRequestSchema(doc, id)
		require.NotNilf(t, schema, "op %q must have an application/json request-body schema", id)
		require.Falsef(t, isBinaryBodySchema(schema),
			"op %q request body must be typed, not the opaque binary RawBody schema", id)
		require.NotEmptyf(t, schema.Ref,
			"op %q request body must be a $ref to the concrete input component, got %+v", id, schema)
		require.Equalf(t, "#/components/schemas/"+wantComponent, schema.Ref,
			"op %q request body must $ref the %q component", id, wantComponent)
	}
}

// TestEveryJSONBodyOperationIsTyped is the completeness gate: NO op that declares an
// application/json request body may publish the opaque binary RawBody schema. The set
// of body ops is derived from the served surface, so every SkipValidateBody site is
// covered without hardcoding a count.
func TestEveryJSONBodyOperationIsTyped(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()

	ids := jsonBodyOperationIDs(doc)
	require.NotEmpty(t, ids, "the document must carry application/json body ops")

	for _, id := range ids {
		schema := jsonRequestSchema(doc, id)
		require.NotNilf(t, schema,
			"op %q must publish an application/json request-body schema", id)
		require.Falsef(t, isBinaryBodySchema(schema),
			"op %q still publishes the opaque binary RawBody schema; its typed request body is not wired", id)
	}
}
