// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

// Package-level Huma per-operation Security metadata. These are SPEC metadata
// ONLY — they populate the generated OpenAPI 3.1 `security` requirement lists so
// the docs advertise how each operation authenticates. Runtime auth is unchanged
// and stays 100% Fiber middleware (guard.With) attached before each Huma
// registration; declaring huma.Operation.Security alters no handler behavior.
//
// The referenced scheme names (BearerAuth, ApiKeyAuth) are declared once on the
// shared Huma API in NewRoutes: BearerAuth via openapi.DeclareBearerAuth and
// ApiKeyAuth via the local nil-guarded SecuritySchemes assignment.

// secBearerOrAPIKey advertises that an operation accepts EITHER a JWT bearer
// token OR an X-API-Key. Two entries in the slice = OR (any one satisfies).
var secBearerOrAPIKey = []map[string][]string{
	{"BearerAuth": {}},
	{"ApiKeyAuth": {}},
}
