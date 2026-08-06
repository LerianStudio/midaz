// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// orgV2Ops enumerates the six organization operations the /v2 group mirrors from /v1:
// the HTTP method, the group-relative op path the Huma document publishes (the shared
// contract prepends "/v2"), and the /v1 operationId the op reuses. The v2 twin is a
// STRAIGHT MIRROR — same handler method, same input/output types — so its operationId
// is the v1 id with the version suffix appended. That suffix is the only thing that
// keeps the two twins from colliding as a duplicate operationId in the one document.
var orgV2Ops = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
}{
	{action: "create", method: http.MethodPost, opPath: "/organizations", v1OperationID: "createOrganization"},
	{action: "list", method: http.MethodGet, opPath: "/organizations", v1OperationID: "listOrganizations"},
	{action: "getByID", method: http.MethodGet, opPath: "/organizations/{id}", v1OperationID: "getOrganizationByID"},
	{action: "update", method: http.MethodPatch, opPath: "/organizations/{id}", v1OperationID: "updateOrganization"},
	{action: "delete", method: http.MethodDelete, opPath: "/organizations/{id}", v1OperationID: "deleteOrganization"},
	{action: "count", method: http.MethodHead, opPath: "/organizations/metrics/count", v1OperationID: "countOrganizations"},
}

// orgV2OperationSuffix is the version suffix a v2 twin appends to its v1 operationId.
// It is spelled literally here rather than read from routeOpSuffixV2 so a rename of the
// production constant surfaces as a contract change the client SDKs would feel, not a
// silently-tracking test.
const orgV2OperationSuffix = "V2"

// operationForMethod projects a Huma path item onto the operation registered for method,
// or nil when the path publishes no such method.
func operationForMethod(item *huma.PathItem, method string) *huma.Operation {
	switch method {
	case http.MethodGet:
		return item.Get
	case http.MethodPost:
		return item.Post
	case http.MethodPatch:
		return item.Patch
	case http.MethodDelete:
		return item.Delete
	case http.MethodHead:
		return item.Head
	case http.MethodPut:
		return item.Put
	default:
		return nil
	}
}

// TestRegisterOrganizationV2Routes_MirrorsV1UnderV2 asserts each of the six organization
// operations is published on the assembled /v2 surface at the /v1 path shape prefixed
// with /v2, advertising the v1 operationId with the version suffix appended. It reads the
// REAL unified document (buildUnifiedHumaAPI) — the same huma.API the served contract and
// the committed dump come from — so it proves the /v2 twin against the mount a client hits.
func TestRegisterOrganizationV2Routes_MirrorsV1UnderV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range orgV2Ops {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v2Key := "/v2" + op.opPath

			item, ok := paths[v2Key]
			require.Truef(t, ok, "the /v2 surface must publish the %s org op at %q", op.action, v2Key)

			operation := operationForMethod(item, op.method)
			require.NotNilf(t, operation, "%s %q must carry a %s operation", op.action, v2Key, op.method)

			assert.Equalf(t, op.v1OperationID+orgV2OperationSuffix, operation.OperationID,
				"the v2 %s org op must advertise the v1 id with the version suffix", op.action)
		})
	}
}

// TestRegisterOrganizationV2Routes_NoDuplicateOperationIDs asserts every operationId in
// the assembled document is unique. huma.OpenAPI.AddOperation panics on a duplicate id, so
// the served contract cannot boot with two ops sharing one — this pins the invariant at the
// document level, catching a v2 twin that dropped its suffix before it becomes a boot panic.
func TestRegisterOrganizationV2Routes_NoDuplicateOperationIDs(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()

	seen := make(map[string]string)

	for key, item := range api.OpenAPI().Paths {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead} {
			operation := operationForMethod(item, method)
			if operation == nil {
				continue
			}

			where := method + " " + key

			prior, dup := seen[operation.OperationID]
			require.Falsef(t, dup, "operationId %q is published twice: %s and %s",
				operation.OperationID, prior, where)

			seen[operation.OperationID] = where
		}
	}
}
