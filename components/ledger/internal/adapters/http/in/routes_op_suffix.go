// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

// Core-ledger operation-ID suffixes. The ledger serves the onboarding + transaction
// families on both the /v1 and /v2 version groups of one OpenAPI document.
// huma.OpenAPI.AddOperation scans the whole document and panics on a duplicate operation
// ID, so a v1 op and its v2 twin — same handler, same path shape under a different
// version prefix — MUST carry distinct operation IDs or the ledger panics at boot. The V2
// suffix makes that disjunction a boot invariant; it secondarily keeps IDs unique across
// the ledger↔tracer hub-spec join. The v1 suffix is empty so the /v1 operation IDs — the
// ones published SDKs already bind to — stay exactly what they were.
//
// These belong to the core-ledger namespace and are deliberately separate from the CRM
// (crmOpSuffixV*) and fee (feeOpSuffixV2) suffixes: each family owns its versioning so a
// change to one cannot ripple into another's operation IDs.
const (
	routeOpSuffixV1 = ""
	routeOpSuffixV2 = "V2"
)

// crmOpSuffixV2 is appended to every CRM operation ID published on the /v2 contract.
// CRM is served only on the /v2 version group, but its operation IDs live in the same
// OpenAPI document as the rest of the ledger surface (and, via the ledger↔tracer
// hub-spec join, alongside the tracer's). huma.OpenAPI.AddOperation scans the whole
// document and panics on a duplicate operation ID, so the V2 suffix keeps the CRM IDs
// distinct within that shared document.
//
// CRM authorizes under the host ledger's "midaz" namespace (routes.go midazName) rather
// than a standalone plugin namespace. That value is the X1 RBAC contract: tenant-manager
// grants migrate from plugin-crm:* to midaz:{holders,instruments}:* at v4 release.
const crmOpSuffixV2 = "V2"
