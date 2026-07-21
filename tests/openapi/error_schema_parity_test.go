// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package openapi holds cross-plane locks over the committed native Huma OAS 3.1
// dumps (components/*/api/openapi.huma.yaml, written by each plane's
// TestOpenAPISpecDump). These tests read the yaml files offline — no server, DB,
// or Docker — so they can run in the same gate that regenerates the specs.
package openapi

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The two committed native Huma OAS 3.1 dumps, relative to this package.
const (
	ledgerSpecPath = "../../components/ledger/api/openapi.huma.yaml"
	tracerSpecPath = "../../components/tracer/api/openapi.huma.yaml"

	// rootErrorSchema is the RFC 9457 problem body Huma emits for every error
	// response. The task-1 schema namer renames problem.Detail -> "Error" on both
	// planes; if it regresses to "Detail" (or is absent), this lock fails first.
	rootErrorSchema = "Error"
)

// refPrefix is how Huma writes internal schema references in the OAS document.
const refPrefix = "#/components/schemas/"

// TestErrorSchemaParity_BothPlanesShareIdenticalErrorContract is the cross-plane
// LOCK the Epic-4.2 check-docs.sh will inherit. It asserts, over the offline dumps:
//
//	(a) both planes expose components.schemas.Error (the namer worked; not "Detail");
//	(b) the Error schema and its full transitive $ref closure are byte-for-byte
//	    identical between ledger and tracer (same schema names, same required[],
//	    same properties, same types);
//	(c) no plane carries an error sub-schema the other lacks (closure sets match).
//
// The closure is computed rather than hardcoded so that a new error sub-schema on
// either plane — or a diverging $ref — trips the lock without a test edit.
func TestErrorSchemaParity_BothPlanesShareIdenticalErrorContract(t *testing.T) {
	ledger := loadSchemas(t, ledgerSpecPath)
	tracer := loadSchemas(t, tracerSpecPath)

	// (a) Both planes must surface "Error" (namer applied), not "Detail".
	require.Contains(t, ledger, rootErrorSchema,
		"ledger spec missing components.schemas.Error — schema namer regressed (Detail not renamed?)")
	require.Contains(t, tracer, rootErrorSchema,
		"tracer spec missing components.schemas.Error — schema namer regressed (Detail not renamed?)")
	require.NotContains(t, ledger, "Detail",
		"ledger spec still has a raw 'Detail' schema — namer did not rename problem.Detail")
	require.NotContains(t, tracer, "Detail",
		"tracer spec still has a raw 'Detail' schema — namer did not rename problem.Detail")

	// (c) The error contract is the transitive closure from "Error". Both planes
	// must reference the exact same set of sub-schemas.
	ledgerClosure := errorClosure(t, ledger)
	tracerClosure := errorClosure(t, tracer)
	require.Equal(t, sortedKeys(ledgerClosure), sortedKeys(tracerClosure),
		"error-schema closure diverges: one plane references an error sub-schema the other lacks")

	// (b) Every schema in the shared closure must be deeply identical between planes.
	for name := range ledgerClosure {
		require.Equal(t, ledger[name], tracer[name],
			"error schema %q diverges between ledger and tracer planes", name)
	}
}

// loadSchemas parses an OAS 3.1 yaml dump and returns components.schemas as a map
// of schema-name -> decoded node. Decoding through yaml into map[string]any gives
// a position-independent structural view, so the compare survives reordering.
func loadSchemas(t *testing.T, path string) map[string]any {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "read %s — has TestOpenAPISpecDump been run for this plane?", path)

	var doc struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &doc), "parse %s as OAS 3.1 yaml", path)
	require.NotEmpty(t, doc.Components.Schemas, "%s has no components.schemas", path)

	return doc.Components.Schemas
}

// errorClosure returns every schema reachable from "Error" via $ref, inclusive.
// It fails the test if a referenced schema is missing from the document.
func errorClosure(t *testing.T, schemas map[string]any) map[string]any {
	t.Helper()

	closure := map[string]any{}
	var visit func(name string)
	visit = func(name string) {
		if _, seen := closure[name]; seen {
			return
		}
		node, ok := schemas[name]
		require.Truef(t, ok, "schema %q is referenced but absent from components.schemas", name)
		closure[name] = node
		for _, ref := range collectRefs(node) {
			visit(ref)
		}
	}
	visit(rootErrorSchema)

	return closure
}

// collectRefs walks a decoded yaml node and returns every internal schema name
// pulled in via "$ref: #/components/schemas/<name>".
func collectRefs(node any) []string {
	var refs []string
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			if k == "$ref" {
				if s, ok := child.(string); ok && strings.HasPrefix(s, refPrefix) {
					refs = append(refs, strings.TrimPrefix(s, refPrefix))
				}
				continue
			}
			refs = append(refs, collectRefs(child)...)
		}
	case []any:
		for _, item := range v {
			refs = append(refs, collectRefs(item)...)
		}
	}
	return refs
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
