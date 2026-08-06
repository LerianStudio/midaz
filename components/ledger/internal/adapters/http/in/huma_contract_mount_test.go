// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"sort"
	"testing"

	"github.com/LerianStudio/lib-auth/v3/auth/middleware"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

// operationsOf returns every declared operation on a PathItem, in a fixed order,
// so a caller can walk the whole served surface without re-listing the eight
// method fields at each call site.
func operationsOf(item *huma.PathItem) []*huma.Operation {
	if item == nil {
		return nil
	}

	candidates := []*huma.Operation{
		item.Get, item.Put, item.Post, item.Delete,
		item.Options, item.Head, item.Patch, item.Trace,
	}

	ops := make([]*huma.Operation, 0, len(candidates))

	for _, op := range candidates {
		if op != nil {
			ops = append(ops, op)
		}
	}

	return ops
}

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

// operationKeys returns the "METHOD path" identity of every operation the document
// declares, sorted, so two documents can be compared for the same served surface.
func operationKeys(doc *huma.OpenAPI) []string {
	keys := make([]string, 0)

	for _, item := range doc.Paths {
		for _, op := range operationsOf(item) {
			keys = append(keys, op.Method+" "+op.Path)
		}
	}

	sort.Strings(keys)

	return keys
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
// per-operation security reference resolves against them — the property the
// committed dump must describe. Each contract is built through the same seam that
// generates the committed dump, so a contract missing the schemes is a dump with
// dangling security references.
func TestUnifiedContractDeclaresSecuritySchemes(t *testing.T) {
	t.Parallel()

	want := []string{"ApiKeyAuth", "BearerAuth"}

	tests := []struct {
		name  string
		build func() *huma.OpenAPI
	}{
		{
			name: "v1",
			build: func() *huma.OpenAPI {
				_, api := buildUnifiedHumaAPI()

				return api.OpenAPI()
			},
		},
		{
			name: "v2",
			build: func() *huma.OpenAPI {
				_, api := buildUnifiedHumaAPIV2()

				return api.OpenAPI()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := tt.build()

			require.Equal(t, want, declaredSecuritySchemes(doc),
				"contract must declare exactly the BearerAuth + ApiKeyAuth security schemes")

			for _, name := range referencedSecuritySchemes(doc) {
				require.Containsf(t, doc.Components.SecuritySchemes, name,
					"operation references security scheme %q that Components.SecuritySchemes does not declare (dangling reference)", name)
			}
		})
	}
}

// TestUnifiedContractIsSingleSourced proves each offline dump harness registers
// exactly the operation set its shared HumaMountDeps mount method produces. The
// harness and a freshly-mounted contract both route through the SAME MountV1/MountV2,
// so a registrar added to a harness body but not the shared method (or the reverse)
// makes the two operation sets diverge — the drift the four hand-maintained mount
// copies used to permit.
func TestUnifiedContractIsSingleSourced(t *testing.T) {
	t.Parallel()

	auth := &middleware.AuthClient{Enabled: false}

	tests := []struct {
		name    string
		harness func() *huma.OpenAPI
		mount   func(fiber.Router, huma.API)
		cfg     openapi.Config
	}{
		{
			name: "v1",
			harness: func() *huma.OpenAPI {
				_, api := buildUnifiedHumaAPI()

				return api.OpenAPI()
			},
			mount: unifiedHumaMountDeps(auth).MountV1,
			cfg: openapi.Config{
				Title:   "Midaz Ledger API",
				Version: "4.0.0",
				Servers: []string{specServerPrefix},
			},
		},
		{
			name: "v2",
			harness: func() *huma.OpenAPI {
				_, api := buildUnifiedHumaAPIV2()

				return api.OpenAPI()
			},
			mount: unifiedHumaMountDeps(auth).MountV2,
			cfg: openapi.Config{
				Title:       "Midaz Ledger API v2",
				Version:     "4.0.0",
				Description: "Midaz Ledger v2 API contract.",
				Servers:     []string{specServerPrefixV2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New()
			group := app.Group(tt.cfg.Servers[0])
			api := AssembleHumaContract(app, group, tt.cfg)
			tt.mount(group, api)

			require.Equal(t, operationKeys(api.OpenAPI()), operationKeys(tt.harness()),
				"%s dump harness must register exactly the shared mount method's operation set", tt.name)
		})
	}
}
