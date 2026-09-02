// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accountExceptionOps enumerates the five account-exception operations both version groups
// publish: the HTTP method, the group-relative op path the Huma document publishes (the
// shared contract prepends "/v1" or "/v2"), the /v1 operationId, and the frozen RBAC verb the
// Fiber guard chain authorizes the method under. The (resource, verb) pair is frozen: the
// resource is always accountExceptionResource and the verb is fixed per method here.
var accountExceptionOps = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
	rbacVerb      string
}{
	{action: "create", method: http.MethodPost, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/exceptions", v1OperationID: "createAccountException", rbacVerb: "post"},
	{action: "list", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/exceptions", v1OperationID: "listAccountExceptions", rbacVerb: "get"},
	{action: "getByID", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/exceptions/{exception_id}", v1OperationID: "getAccountExceptionByID", rbacVerb: "get"},
	{action: "update", method: http.MethodPatch, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/exceptions/{exception_id}", v1OperationID: "updateAccountException", rbacVerb: "patch"},
	{action: "delete", method: http.MethodDelete, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}/exceptions/{exception_id}", v1OperationID: "deleteAccountException", rbacVerb: "delete"},
}

// accountExceptionV2OperationSuffix is the version suffix a v2 twin appends to its v1
// operationId. Spelled literally rather than read from v2OpSuffix so a rename of the
// production constant surfaces as a contract change, not a silently-tracking test.
const accountExceptionV2OperationSuffix = "V2"

// accountExceptionJSONMediaType is the content type the ops publish their bodies under.
const accountExceptionJSONMediaType = "application/json"

// TestAccountExceptionResource_Frozen pins the RBAC resource constant. The pair
// ("account-exceptions", <verb>) is a contract with the Access Manager: the routes fail closed
// with 403 until the resource is registered there, so a silent rename would break authorization
// without any other gate noticing.
func TestAccountExceptionResource_Frozen(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "account-exceptions", accountExceptionResource,
		"the account-exception RBAC resource is frozen; renaming it silently breaks Access Manager authorization")
}

// TestRegisterAccountExceptionRoutes_PublishedOnBothVersions asserts each of the five
// account-exception operations is published on BOTH the /v1 and /v2 surfaces of the REAL
// unified document, at the /v1 path shape prefixed with the version. Reading the assembled
// document also proves the mount boots without a duplicate-operationId panic: huma.AddOperation
// panics on a collision, so buildUnifiedHumaAPI returning at all is the no-panic evidence.
func TestRegisterAccountExceptionRoutes_PublishedOnBothVersions(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range accountExceptionOps {
		for _, prefix := range []string{"/v1", "/v2"} {
			t.Run(prefix+"/"+op.action, func(t *testing.T) {
				t.Parallel()

				key := prefix + op.opPath

				item, ok := paths[key]
				require.Truef(t, ok, "the %s surface must publish the %s account-exception op at %q", prefix, op.action, key)

				operation := operationForMethod(item, op.method)
				require.NotNilf(t, operation, "%s %q must carry a %s operation", op.action, key, op.method)

				wantID := op.v1OperationID
				if prefix == "/v2" {
					wantID = op.v1OperationID + accountExceptionV2OperationSuffix
				}

				assert.Equalf(t, wantID, operation.OperationID,
					"the %s %s account-exception op must advertise operationId %q", prefix, op.action, wantID)
			})
		}
	}
}

// TestRegisterAccountExceptionV2Routes_MirrorsV1UnderV2 asserts each op is published on the
// /v2 surface at the /v1 path shape prefixed with /v2, advertising the v1 operationId with the
// version suffix appended.
func TestRegisterAccountExceptionV2Routes_MirrorsV1UnderV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range accountExceptionOps {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v2Key := "/v2" + op.opPath

			item, ok := paths[v2Key]
			require.Truef(t, ok, "the /v2 surface must publish the %s account-exception op at %q", op.action, v2Key)

			operation := operationForMethod(item, op.method)
			require.NotNilf(t, operation, "%s %q must carry a %s operation", op.action, v2Key, op.method)

			assert.Equalf(t, op.v1OperationID+accountExceptionV2OperationSuffix, operation.OperationID,
				"the v2 %s account-exception op must advertise the v1 id with the version suffix", op.action)
		})
	}
}

// accountExceptionReferencedComponents gathers the base component names the ops name in their
// 2xx JSON response bodies, read off the assembled document so a rename of the model is followed
// here automatically. It walks both /v1 and /v2 twins; a straight mirror names the identical set.
func accountExceptionReferencedComponents(paths map[string]*huma.PathItem) map[string]bool {
	const refPrefix = "#/components/schemas/"

	refs := make(map[string]bool)

	collect := func(ref string) {
		if name, ok := strings.CutPrefix(ref, refPrefix); ok {
			refs[name] = true
		}
	}

	for _, op := range accountExceptionOps {
		for _, prefix := range []string{"/v1", "/v2"} {
			item, ok := paths[prefix+op.opPath]
			if !ok {
				continue
			}

			operation := operationForMethod(item, op.method)
			if operation == nil {
				continue
			}

			for status, resp := range operation.Responses {
				if !strings.HasPrefix(status, "2") {
					continue
				}

				if media, ok := resp.Content[accountExceptionJSONMediaType]; ok && media.Schema != nil {
					collect(media.Schema.Ref)
				}
			}
		}
	}

	return refs
}

// TestRegisterAccountExceptionV2Routes_ReusesV1SchemaComponents proves the /v2 twin REUSES the
// v1 response Go type: Huma dedups it to ONE schema component, so no AccountException schema
// component carries the version suffix. Were a v2 twin to mint its own type, a suffixed
// component would appear and the guard below would turn red.
func TestRegisterAccountExceptionV2Routes_ReusesV1SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()
	schemas := doc.Components.Schemas.Map()

	referenced := accountExceptionReferencedComponents(doc.Paths)
	require.Containsf(t, referenced, "AccountException",
		"the account-exception ops must reference the AccountException body component, or this test guards nothing")

	for name := range referenced {
		assert.NotContainsf(t, schemas, name+accountExceptionV2OperationSuffix,
			"no %s twin of the reused account-exception component %q may be minted", accountExceptionV2OperationSuffix, name)
	}
}
