// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"sort"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/require"
)

// referencedSecuritySchemes collects the DISTINCT security-scheme names every
// operation (and the document-level Security) references across the whole
// document. A name here that is absent from Components.SecuritySchemes is a
// dangling reference: the served spec points at a scheme it never declares.
func referencedSecuritySchemes(doc *huma.OpenAPI) []string {
	seen := map[string]bool{}

	collect := func(requirements []map[string][]string) {
		for _, requirement := range requirements {
			for name := range requirement {
				seen[name] = true
			}
		}
	}

	collect(doc.Security)

	for _, item := range doc.Paths {
		for _, op := range operationsOf(item) {
			collect(op.Security)
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// declaredSecuritySchemes returns the sorted names Components.SecuritySchemes declares.
func declaredSecuritySchemes(doc *huma.OpenAPI) []string {
	if doc.Components == nil || doc.Components.SecuritySchemes == nil {
		return nil
	}

	names := make([]string, 0, len(doc.Components.SecuritySchemes))
	for name := range doc.Components.SecuritySchemes {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// TestUnifiedContractDeclaresSecuritySchemes proves the harness-produced ledger
// contract carries the BearerAuth + ApiKeyAuth security schemes and that every
// per-operation security reference resolves against them — the property the committed
// dump must describe. The single document carries both the /v1 and /v2 operations, so
// one contract built through the dump seam covers the whole surface; a contract missing
// the schemes is a dump with dangling security references.
func TestUnifiedContractDeclaresSecuritySchemes(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()

	require.Equal(t, []string{"ApiKeyAuth", "BearerAuth"}, declaredSecuritySchemes(doc),
		"contract must declare exactly the BearerAuth + ApiKeyAuth security schemes")

	for _, name := range referencedSecuritySchemes(doc) {
		require.Containsf(t, doc.Components.SecuritySchemes, name,
			"operation references security scheme %q that Components.SecuritySchemes does not declare (dangling reference)", name)
	}
}
