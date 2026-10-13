// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// transactionRouteV2Ops enumerates the five transaction-route operations the /v2 group mirrors
// from /v1: the HTTP method, the group-relative op path the Huma document publishes (the
// shared contract prepends "/v2"), and the /v1 operationId the op reuses. transaction-route has
// no HEAD-count op, so the CRUD-plus-list set is the whole surface. The v2 twin is a STRAIGHT
// MIRROR — same handler method, same input/output types — so its operationId is the v1 id with
// the version suffix appended. That suffix is the only thing that keeps the two twins from
// colliding as a duplicate operationId in the one document.
var transactionRouteV2Ops = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
}{
	{action: "create", method: http.MethodPost, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes", v1OperationID: "createTransactionRoute"},
	{action: "list", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes", v1OperationID: "listTransactionRoutes"},
	{action: "getByID", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id}", v1OperationID: "getTransactionRouteByID"},
	{action: "update", method: http.MethodPatch, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id}", v1OperationID: "updateTransactionRoute"},
	{action: "delete", method: http.MethodDelete, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id}", v1OperationID: "deleteTransactionRoute"},
}

// transactionRouteV2OperationSuffix is the version suffix a v2 twin appends to its v1
// operationId. It is spelled literally here rather than read from v2OpSuffix so a rename
// of the production constant surfaces as a contract change the client SDKs would feel, not a
// silently-tracking test.
const transactionRouteV2OperationSuffix = "V2"

// transactionRouteJSONMediaType is the content type the transaction-route ops publish their
// request and response bodies under. The reuse invariant below is asserted only over these bodies.
const transactionRouteJSONMediaType = "application/json"

// TestRegisterTransactionRouteV2Routes_MirrorsV1UnderV2 asserts each of the five transaction-route
// operations is published on the assembled /v2 surface at the /v1 path shape prefixed with /v2,
// advertising the v1 operationId with the version suffix appended. It reads the REAL unified
// document (buildUnifiedHumaAPI) — the same huma.API the served contract and the committed dump
// come from — so it proves the /v2 twin against the mount a client hits.
func TestRegisterTransactionRouteV2Routes_MirrorsV1UnderV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range transactionRouteV2Ops {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v2Key := "/v2" + op.opPath

			item, ok := paths[v2Key]
			require.Truef(t, ok, "the /v2 surface must publish the %s transaction-route op at %q", op.action, v2Key)

			operation := operationForMethod(item, op.method)
			require.NotNilf(t, operation, "%s %q must carry a %s operation", op.action, v2Key, op.method)

			assert.Equalf(t, op.v1OperationID+transactionRouteV2OperationSuffix, operation.OperationID,
				"the v2 %s transaction-route op must advertise the v1 id with the version suffix", op.action)
		})
	}
}

// transactionRouteOpBodyRefs projects an operation onto the component $refs its JSON request body
// and its 2xx JSON response body name. A body Huma describes inline (the opaque RawBody request
// schema the create/update ops carry) has no $ref, so its slot comes back "". Returning the refs
// — not the schemas — is the point: reuse of a v1 Go type is observable precisely as the v2 twin
// pointing at the SAME "#/components/schemas/<Name>" string.
func transactionRouteOpBodyRefs(op *huma.Operation) (reqRef string, respRefs []string) {
	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content[transactionRouteJSONMediaType]; ok && media.Schema != nil {
			reqRef = media.Schema.Ref
		}
	}

	for status, resp := range op.Responses {
		if !strings.HasPrefix(status, "2") {
			continue
		}

		if media, ok := resp.Content[transactionRouteJSONMediaType]; ok && media.Schema != nil {
			respRefs = append(respRefs, media.Schema.Ref)
		}
	}

	return reqRef, respRefs
}

// transactionRouteReferencedComponents gathers the base component names ("#/components/schemas/"
// prefix stripped) the transaction-route ops name in their JSON bodies, read off the assembled
// document so a rename of a transaction-route type is followed here automatically. It walks both
// /v1 and /v2 twins; a straight mirror names the identical set on each side.
func transactionRouteReferencedComponents(paths map[string]*huma.PathItem) map[string]bool {
	const refPrefix = "#/components/schemas/"

	refs := make(map[string]bool)

	collect := func(ref string) {
		if name, ok := strings.CutPrefix(ref, refPrefix); ok {
			refs[name] = true
		}
	}

	for _, op := range transactionRouteV2Ops {
		for _, prefix := range []string{"/v1", "/v2"} {
			item, ok := paths[prefix+op.opPath]
			if !ok {
				continue
			}

			operation := operationForMethod(item, op.method)
			if operation == nil {
				continue
			}

			reqRef, respRefs := transactionRouteOpBodyRefs(operation)

			collect(reqRef)

			for _, r := range respRefs {
				collect(r)
			}
		}
	}

	return refs
}

// transactionRouteComponent is the schema component a $ref names, from the assembled document.
func transactionRouteComponent(t *testing.T, api huma.API, ref string) *huma.Schema {
	t.Helper()

	name, ok := strings.CutPrefix(ref, "#/components/schemas/")
	require.Truef(t, ok, "unexpected ref %q", ref)

	schema, ok := api.OpenAPI().Components.Schemas.Map()[name]
	require.Truef(t, ok, "the document must publish the %s component", name)

	return schema
}

// The /v1 transaction-route contract is the one v1 clients already bind to: its
// ops keep naming the canonical TransactionRoute, CreateTransactionRouteInput and
// UpdateTransactionRouteInput components, and none of those carries the /v2-only
// optionalOperationRoutes list.
func TestRegisterTransactionRouteRoutes_V1PublishesNoOptionalLinks(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	want := map[string]struct{ request, response string }{
		"create":  {"#/components/schemas/CreateTransactionRouteInput", "#/components/schemas/TransactionRoute"},
		"getByID": {"", "#/components/schemas/TransactionRoute"},
		"update":  {"#/components/schemas/UpdateTransactionRouteInput", "#/components/schemas/TransactionRoute"},
	}

	for _, op := range transactionRouteV2Ops {
		expected, ok := want[op.action]
		if !ok {
			continue
		}

		item, ok := paths["/v1"+op.opPath]
		require.Truef(t, ok, "the /v1 surface must publish the %s transaction-route op", op.action)

		v1Op := operationForMethod(item, op.method)
		require.NotNilf(t, v1Op, "the v1 %s transaction-route op must carry a %s operation", op.action, op.method)

		request, responses := transactionRouteOpBodyRefs(v1Op)
		assert.Equalf(t, expected.request, request, "the v1 %s request component", op.action)
		assert.Containsf(t, responses, expected.response, "the v1 %s response component", op.action)

		for _, ref := range append(responses, request) {
			if ref == "" {
				continue
			}

			_, hasOptional := transactionRouteComponent(t, api, ref).Properties["optionalOperationRoutes"]
			assert.Falsef(t, hasOptional, "the v1 %s component %s must not publish optionalOperationRoutes", op.action, ref)
		}
	}
}

// The /v2 transaction-route contract, at ledger and at organization level, takes
// and answers the optional links as a sibling list of operationRoutes.
func TestRegisterTransactionRouteV2Routes_PublishesOptionalLinks(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	ops := []struct {
		name, method, path string
		hasRequest         bool
	}{
		{"create", http.MethodPost, "/v2/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes", true},
		{"getByID", http.MethodGet, "/v2/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id}", false},
		{"update", http.MethodPatch, "/v2/organizations/{organization_id}/ledgers/{ledger_id}/transaction-routes/{transaction_route_id}", true},
		{"createOrganization", http.MethodPost, "/v2/organizations/{organization_id}/transaction-routes", true},
		{"getOrganization", http.MethodGet, "/v2/organizations/{organization_id}/transaction-routes/{transaction_route_id}", false},
		{"updateOrganization", http.MethodPatch, "/v2/organizations/{organization_id}/transaction-routes/{transaction_route_id}", true},
	}

	for _, op := range ops {
		item, ok := paths[op.path]
		require.Truef(t, ok, "the /v2 surface must publish %s", op.path)

		v2Op := operationForMethod(item, op.method)
		require.NotNilf(t, v2Op, "%s must carry a %s operation", op.path, op.method)

		request, responses := transactionRouteOpBodyRefs(v2Op)

		if op.hasRequest {
			require.NotEmptyf(t, request, "the v2 %s op must name a request component", op.name)

			optional, hasOptional := transactionRouteComponent(t, api, request).Properties["optionalOperationRoutes"]
			require.Truef(t, hasOptional, "the v2 %s request %s must accept optionalOperationRoutes", op.name, request)
			assert.Truef(t, optional.Nullable || slices.Contains(schemaTypes(optional), "null"),
				"the v2 %s request %s must declare optionalOperationRoutes nullable: null keeps the stored list", op.name, request)
		}

		require.NotEmptyf(t, responses, "the v2 %s op must name a response component", op.name)

		for _, ref := range responses {
			properties := transactionRouteComponent(t, api, ref).Properties

			_, hasRequired := properties["operationRoutes"]
			_, hasOptional := properties["optionalOperationRoutes"]
			assert.Truef(t, hasRequired && hasOptional, "the v2 %s response %s must answer both link lists", op.name, ref)
		}
	}
}

// schemaTypes is the JSON type list of a schema, whether published as one type or several.
func schemaTypes(schema *huma.Schema) []string {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}

	var decoded struct {
		Type any `json:"type"`
	}

	if json.Unmarshal(data, &decoded) != nil {
		return nil
	}

	switch typed := decoded.Type.(type) {
	case string:
		return []string{typed}
	case []any:
		types := make([]string, 0, len(typed))
		for _, item := range typed {
			if name, ok := item.(string); ok {
				types = append(types, name)
			}
		}

		return types
	}

	return nil
}
