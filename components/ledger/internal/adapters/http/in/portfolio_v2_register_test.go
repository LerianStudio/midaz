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

// portfolioV2Ops enumerates the six portfolio operations the /v2 group mirrors from /v1:
// the HTTP method, the group-relative op path the Huma document publishes (the shared
// contract prepends "/v2"), and the /v1 operationId the op reuses. Unlike account-type,
// portfolio carries a HEAD-count op, so the CRUD-plus-list set is joined by the metrics
// count. The v2 twin is a STRAIGHT MIRROR — same handler method, same input/output types —
// so its operationId is the v1 id with the version suffix appended. That suffix is the only
// thing that keeps the two twins from colliding as a duplicate operationId in the one
// document.
var portfolioV2Ops = []struct {
	action        string
	method        string
	opPath        string
	v1OperationID string
}{
	{action: "create", method: http.MethodPost, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/portfolios", v1OperationID: "createPortfolio"},
	{action: "list", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/portfolios", v1OperationID: "listPortfolios"},
	{action: "getByID", method: http.MethodGet, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", v1OperationID: "getPortfolioByID"},
	{action: "update", method: http.MethodPatch, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", v1OperationID: "updatePortfolio"},
	{action: "delete", method: http.MethodDelete, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/{id}", v1OperationID: "deletePortfolio"},
	{action: "count", method: http.MethodHead, opPath: "/organizations/{organization_id}/ledgers/{ledger_id}/portfolios/metrics/count", v1OperationID: "countPortfolios"},
}

// portfolioV2OperationSuffix is the version suffix a v2 twin appends to its v1 operationId.
// It is spelled literally here rather than read from routeOpSuffixV2 so a rename of the
// production constant surfaces as a contract change the client SDKs would feel, not a
// silently-tracking test.
const portfolioV2OperationSuffix = "V2"

// portfolioJSONMediaType is the content type the portfolio ops publish their request and
// response bodies under. The reuse invariant below is asserted only over these bodies.
const portfolioJSONMediaType = "application/json"

// TestRegisterPortfolioV2Routes_MirrorsV1UnderV2 asserts each of the six portfolio
// operations is published on the assembled /v2 surface at the /v1 path shape prefixed with
// /v2, advertising the v1 operationId with the version suffix appended. It reads the REAL
// unified document (buildUnifiedHumaAPI) — the same huma.API the served contract and the
// committed dump come from — so it proves the /v2 twin against the mount a client hits.
func TestRegisterPortfolioV2Routes_MirrorsV1UnderV2(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	for _, op := range portfolioV2Ops {
		t.Run(op.action, func(t *testing.T) {
			t.Parallel()

			v2Key := "/v2" + op.opPath

			item, ok := paths[v2Key]
			require.Truef(t, ok, "the /v2 surface must publish the %s portfolio op at %q", op.action, v2Key)

			operation := operationForMethod(item, op.method)
			require.NotNilf(t, operation, "%s %q must carry a %s operation", op.action, v2Key, op.method)

			assert.Equalf(t, op.v1OperationID+portfolioV2OperationSuffix, operation.OperationID,
				"the v2 %s portfolio op must advertise the v1 id with the version suffix", op.action)
		})
	}
}

// portfolioOpBodyRefs projects an operation onto the component $refs its JSON request body
// and its 2xx JSON response body name. A body Huma describes inline (the opaque RawBody
// request schema the create/update ops carry) has no $ref, so its slot comes back "". The
// HEAD-count op carries neither a JSON request body nor a JSON response body (its output is
// headers only), so both its slots come back "" too. Returning the refs — not the schemas —
// is the point: reuse of a v1 Go type is observable precisely as the v2 twin pointing at the
// SAME "#/components/schemas/<Name>" string.
func portfolioOpBodyRefs(op *huma.Operation) (reqRef string, respRefs []string) {
	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content[portfolioJSONMediaType]; ok && media.Schema != nil {
			reqRef = media.Schema.Ref
		}
	}

	for status, resp := range op.Responses {
		if !strings.HasPrefix(status, "2") {
			continue
		}

		if media, ok := resp.Content[portfolioJSONMediaType]; ok && media.Schema != nil {
			respRefs = append(respRefs, media.Schema.Ref)
		}
	}

	return reqRef, respRefs
}

// portfolioReferencedComponents gathers the base component names ("#/components/schemas/"
// prefix stripped) the portfolio ops name in their JSON bodies, read off the assembled
// document so a rename of a portfolio type is followed here automatically. It walks both
// /v1 and /v2 twins; a straight mirror names the identical set on each side.
func portfolioReferencedComponents(paths map[string]*huma.PathItem) map[string]bool {
	const refPrefix = "#/components/schemas/"

	refs := make(map[string]bool)

	collect := func(ref string) {
		if name, ok := strings.CutPrefix(ref, refPrefix); ok {
			refs[name] = true
		}
	}

	for _, op := range portfolioV2Ops {
		for _, prefix := range []string{"/v1", "/v2"} {
			item, ok := paths[prefix+op.opPath]
			if !ok {
				continue
			}

			operation := operationForMethod(item, op.method)
			if operation == nil {
				continue
			}

			reqRef, respRefs := portfolioOpBodyRefs(operation)

			collect(reqRef)

			for _, r := range respRefs {
				collect(r)
			}
		}
	}

	return refs
}

// TestRegisterPortfolioV2Routes_ReusesV1SchemaComponents proves the core correctness claim
// of the straight-mirror approach: the /v2 portfolio twin REUSES the v1 request/response Go
// types, so Huma's registry dedups them to ONE schema component and the v2 op's body $ref is
// byte-identical to the v1 op's. It reads the REAL unified document, the same huma.API the
// served contract and the committed dump come from.
//
// Were a v2 twin to mint its own type for any body, its op would $ref a different (V2-named)
// component and the equality below would turn red.
func TestRegisterPortfolioV2Routes_ReusesV1SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	paths := api.OpenAPI().Paths

	// Guards the assertions below against vacuously passing on a document where every ref
	// came back "": at least one portfolio op must actually name a response-body component.
	sawSharedResponseRef := false

	for _, op := range portfolioV2Ops {
		v1Item, ok := paths["/v1"+op.opPath]
		require.Truef(t, ok, "the /v1 surface must publish the %s portfolio op", op.action)

		v2Item, ok := paths["/v2"+op.opPath]
		require.Truef(t, ok, "the /v2 surface must publish the %s portfolio op", op.action)

		v1Op := operationForMethod(v1Item, op.method)
		require.NotNilf(t, v1Op, "the v1 %s portfolio op must carry a %s operation", op.action, op.method)

		v2Op := operationForMethod(v2Item, op.method)
		require.NotNilf(t, v2Op, "the v2 %s portfolio op must carry a %s operation", op.action, op.method)

		v1Req, v1Resp := portfolioOpBodyRefs(v1Op)
		v2Req, v2Resp := portfolioOpBodyRefs(v2Op)

		assert.Equalf(t, v1Req, v2Req,
			"the v2 %s portfolio op must name the SAME request-body schema as v1 (a straight mirror mints no new request type)", op.action)
		assert.ElementsMatchf(t, v1Resp, v2Resp,
			"the v2 %s portfolio op must name the SAME response-body component(s) as v1 (Huma dedups the reused Go type to one schema)", op.action)

		for _, ref := range v2Resp {
			if ref == "" {
				continue
			}

			sawSharedResponseRef = true

			assert.Falsef(t, strings.HasSuffix(ref, portfolioV2OperationSuffix),
				"the v2 %s portfolio op response ref %q must not name a %s-suffixed component — the v1 type is reused, not re-minted",
				op.action, ref, portfolioV2OperationSuffix)
		}
	}

	require.True(t, sawSharedResponseRef,
		"at least one portfolio op must reference a response-body component, or the reuse claim is vacuous")
}

// TestRegisterPortfolioV2Routes_MintsNoV2SchemaComponents guards against accidental new-type
// creation for the straight mirror: no portfolio schema component may carry the version
// suffix. TransactionV2 is a legitimate component (transaction v2 is NOT a straight mirror
// and DOES introduce its own types); the portfolio mirror must add no such twin.
func TestRegisterPortfolioV2Routes_MintsNoV2SchemaComponents(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()
	schemas := doc.Components.Schemas.Map()

	// The components the portfolio ops actually name, gathered from the assembled document
	// rather than hardcoded, so a renamed portfolio type is followed here. The reused v1
	// type is registered ONCE, so the suffixed twin of each must be absent.
	referenced := portfolioReferencedComponents(doc.Paths)
	require.Containsf(t, referenced, "Portfolio",
		"the portfolio ops must reference the Portfolio body component, or this test guards nothing")

	for name := range referenced {
		assert.NotContainsf(t, schemas, name+portfolioV2OperationSuffix,
			"no %s twin of the reused portfolio body component %q may be minted", portfolioV2OperationSuffix, name)
	}

	// The document-wide guard: no portfolio-named schema carries the V2 suffix.
	for name := range schemas {
		if !strings.HasPrefix(name, "Portfolio") {
			continue
		}

		assert.Falsef(t, strings.HasSuffix(name, portfolioV2OperationSuffix),
			"no portfolio schema component may carry the %s suffix; found %q", portfolioV2OperationSuffix, name)
	}
}
