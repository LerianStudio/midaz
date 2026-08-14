// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// retiredAssetRateV2Ops enumerates the three asset-rate operations the /v2 group no longer
// publishes: a PUT create-or-upsert, a GET by external id, and a cursor-paginated GET list
// keyed by a free-form asset code. Each row carries the group-relative op path and the v1
// operationId; the retired v2 twin was that id with the "V2" suffix appended.
var retiredAssetRateV2Ops = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
}{
	{action: "createOrUpdate", method: http.MethodPut, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates", v1OperationID: "createOrUpdateAssetRate"},
	{action: "getByExternalID", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/{external_id}", v1OperationID: "getAssetRateByExternalID"},
	{action: "listByAssetCode", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/asset-rates/from/{asset_code}", v1OperationID: "getAllAssetRatesByAssetCode"},
}

// retiredAssetRateV2Suffix is the version suffix the retired v2 twin appended to its v1
// operationId. Spelled literally so a rename of the production constant cannot make this
// guard silently track it.
const retiredAssetRateV2Suffix = "V2"

// TestRegisterAssetRateV2Routes_Retired proves the v2 asset-rate surface is gone while v1
// stays intact. It reads the REAL unified document (buildUnifiedHumaAPI) — the same huma.API
// the served contract and the committed dump come from — and asserts, for each of the three
// asset-rate operations, that the /v2 path key carries no such operation while the /v1 twin
// still publishes its unsuffixed v1 operationId.
func TestRegisterAssetRateV2Routes_Retired(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range retiredAssetRateV2Ops {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			// The /v2 twin is retired: either the path key is absent, or it carries no such
			// method. Either way the client can no longer reach the op on /v2.
			if item, ok := paths["/v2"+op.opPath]; ok {
				assert.Nilf(t, operationForMethod(item, op.method),
					"the /v2 surface must no longer publish the %s asset-rate op", op.action)
			}

			// The /v1 twin is untouched: same path, same unsuffixed v1 operationId.
			v1Item, ok := paths["/v1"+op.opPath]
			require.Truef(t, ok, "the /v1 surface must still publish the %s asset-rate op", op.action)

			v1Op := operationForMethod(v1Item, op.method)
			require.NotNilf(t, v1Op, "the v1 %s asset-rate op must carry a %s operation", op.action, op.method)

			assert.Equalf(t, op.v1OperationID, v1Op.OperationID,
				"the v1 %s asset-rate op keeps its unsuffixed operationId", op.action)
		})
	}
}

// TestRegisterAssetRateV2Routes_NoV2OperationIDs asserts the document publishes no asset-rate
// operationId carrying the version suffix, catching a v2 asset-rate op re-registered under any
// path shape.
func TestRegisterAssetRateV2Routes_NoV2OperationIDs(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()

	retired := map[string]bool{}
	for _, op := range retiredAssetRateV2Ops {
		retired[op.v1OperationID+retiredAssetRateV2Suffix] = true
	}

	for _, item := range api.OpenAPI().Paths {
		for _, operation := range operationsOf(item) {
			assert.NotContainsf(t, retired, operation.OperationID,
				"a retired v2 asset-rate op is still registered: %q", operation.OperationID)
		}
	}
}
