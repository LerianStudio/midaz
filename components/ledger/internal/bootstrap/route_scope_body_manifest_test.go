// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	ledgerembed "github.com/LerianStudio/midaz/v4/components/ledger"
)

// ledgerSpecPath is the committed Huma OAS 3.1 dump of the ledger, kept equal to the
// served contract by the spec drift gate.
const ledgerSpecPath = "../../api/openapi.huma.yaml"

// bodyScopeCoordinates is the LOCKED set of dimensions a request body may name
// instead of the path: the organization and the ledger a request acts in. Every
// operation whose request schema carries one of them, on a path that does not, must
// declare where in the body it is read.
var bodyScopeCoordinates = map[string]bool{
	"organizationId": true,
	"ledgerId":       true,
}

// bodyPointerDimensions is the LOCKED set of dimensions a write body may point at
// besides its coordinates: the portfolio, segment, holder or account a new or
// updated record is attached to. They are declared per route and pinned by the
// manifest test of the ledger package.
var bodyPointerDimensions = map[string]bool{
	"portfolioId": true,
	"segmentId":   true,
	"holderId":    true,
	"accountId":   true,
}

// manifestScopeRoutes parses the scope.routes of the embedded manifest.
func manifestScopeRoutes(t *testing.T) []declaration.DeclarationScopeRoute {
	t.Helper()

	var manifest declaration.DeclarationManifest

	require.NoError(t, yaml.Unmarshal(ledgerembed.MidazManifest, &manifest), "the embedded manifest must parse")
	require.NoError(t, manifest.Validate(), "the embedded manifest must validate")
	require.NotNil(t, manifest.Scope, "the embedded manifest must declare a scope section")

	return manifest.Scope.Routes
}

// scopeRouteKey is a declared route's identity in routeRow.key form.
func scopeRouteKey(r declaration.DeclarationScopeRoute) string {
	return strings.ToUpper(r.Method) + "\t" + r.Path
}

// bodyFieldsByRoute renders scope.routes as route key -> coordinate name -> the body
// fields it is read from, sorted. Only body scope coordinates are rendered.
func bodyFieldsByRoute(routes []declaration.DeclarationScopeRoute) map[string]map[string][]string {
	out := make(map[string]map[string][]string, len(routes))

	for _, r := range routes {
		key := scopeRouteKey(r)

		for _, d := range r.Dimensions {
			if d.From != "body" || !bodyScopeCoordinates[d.Name] {
				continue
			}

			if out[key] == nil {
				out[key] = make(map[string][]string)
			}

			out[key][d.Name] = append(out[key][d.Name], d.Field)
		}

		for name := range out[key] {
			sort.Strings(out[key][name])
		}
	}

	return out
}

// specSchemaWalker collects the body field paths, in the scope field syntax, at
// which a request schema carries a property named as a body scope coordinate.
type specSchemaWalker struct {
	schemas map[string]any
	found   map[string][]string
}

func (w *specSchemaWalker) walk(node any, path string, seen map[string]bool) {
	schema, ok := node.(map[string]any)
	if !ok {
		return
	}

	if ref, isRef := schema["$ref"].(string); isRef {
		name := ref[strings.LastIndex(ref, "/")+1:]
		if seen[name] {
			return
		}

		next := make(map[string]bool, len(seen)+1)
		for k := range seen {
			next[k] = true
		}

		next[name] = true

		w.walk(w.schemas[name], path, next)

		return
	}

	for _, combinator := range []string{"allOf", "oneOf", "anyOf"} {
		if branches, has := schema[combinator].([]any); has {
			for _, branch := range branches {
				w.walk(branch, path, seen)
			}
		}
	}

	if items, has := schema["items"]; has {
		w.walk(items, path+"[]", seen)
	}

	properties, _ := schema["properties"].(map[string]any)
	for name, property := range properties {
		field := name
		if path != "" {
			field = path + "." + name
		}

		if bodyScopeCoordinates[name] {
			w.found[name] = append(w.found[name], field)
		}

		w.walk(property, field, seen)
	}
}

// specBodyCoordinateFields reads the committed spec and returns, per operation whose
// request body names a body scope coordinate that its path does not carry, the
// fields each coordinate sits at — route key -> dimension name -> sorted fields.
func specBodyCoordinateFields(t *testing.T, dims []declaration.DeclarationDimension) map[string]map[string][]string {
	t.Helper()

	raw, err := os.ReadFile(ledgerSpecPath)
	require.NoError(t, err, "the committed ledger spec must be readable")

	var spec struct {
		Paths      map[string]map[string]any `yaml:"paths"`
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}

	require.NoError(t, yaml.Unmarshal(raw, &spec))
	require.NotEmpty(t, spec.Paths, "the committed spec must declare paths")

	paramOf := make(map[string]string, len(dims))
	for _, dim := range dims {
		paramOf[dim.Name] = dim.Param
	}

	out := make(map[string]map[string][]string)

	for specPath, item := range spec.Paths {
		fiberPath := strings.NewReplacer("{", ":", "}", "").Replace(specPath)
		params := make(map[string]bool)

		for _, name := range routePathParams(fiberPath) {
			params[name] = true
		}

		for method, rawOp := range item {
			op, isOp := rawOp.(map[string]any)
			if !isOp {
				continue
			}

			body, hasBody := op["requestBody"].(map[string]any)
			if !hasBody {
				continue
			}

			content, _ := body["content"].(map[string]any)
			walker := &specSchemaWalker{schemas: spec.Components.Schemas, found: make(map[string][]string)}

			for _, media := range content {
				if typed, isTyped := media.(map[string]any); isTyped {
					walker.walk(typed["schema"], "", map[string]bool{})
				}
			}

			for name, fields := range walker.found {
				if params[paramOf[name]] {
					continue
				}

				key := strings.ToUpper(method) + "\t" + fiberPath
				if out[key] == nil {
					out[key] = make(map[string][]string)
				}

				out[key][name] = append(out[key][name], fields...)
				sort.Strings(out[key][name])
			}
		}
	}

	return out
}

// TestManifestScope_BodyRoutesAreGuardedRoutes ties every scope.routes entry to a
// route the unified server really mounts, with that method and that path, behind
// the authorization guard. A declaration for a path nothing serves — a typo, a
// version prefix left out — is read by no request and protects nothing.
func TestManifestScope_BodyRoutesAreGuardedRoutes(t *testing.T) {
	unsetDocsGate(t)

	routes := manifestScopeRoutes(t)
	require.NotEmpty(t, routes, "the manifest must declare the routes that read their scope from the body")

	server := buildFullSurfaceServer(t)

	mounted := make(map[string]bool)
	for _, group := range groupRouteRows(collectRouteRows(t, server.app)) {
		mounted[group.key] = true
	}

	for _, r := range routes {
		key := scopeRouteKey(r)

		assert.Truef(t, mounted[key], "scope.routes declares %s, which the unified server does not mount", r.Method+" "+r.Path)
		assert.Falsef(t, unguardedPublicRoutes[key], "scope.routes declares %s, which is mounted outside the guard", r.Method+" "+r.Path)

		for _, d := range r.Dimensions {
			switch d.From {
			case "body":
				assert.Truef(t, bodyScopeCoordinates[d.Name] || bodyPointerDimensions[d.Name],
					"%s reads %s from the body, which is neither a body scope coordinate nor a body pointer",
					r.Method+" "+r.Path, d.Name)
			case "query":
				assert.Equalf(t, "GET", strings.ToUpper(r.Method),
					"%s reads %s from the query, which only a list filter may carry", r.Method+" "+r.Path, d.Name)
			default:
				assert.Failf(t, "undeclared carrier", "%s reads %s from %q", r.Method+" "+r.Path, d.Name, d.From)
			}
		}
	}
}

// TestManifestScope_EveryBodyCarriedCoordinateIsDeclared reads the committed contract
// and requires the manifest to declare EXACTLY the body fields at which an operation
// names its organization or ledger outside the path. A route left undeclared answers
// a partner with a refusal it cannot get past; a field misspelled is refused as
// missing on every request.
func TestManifestScope_EveryBodyCarriedCoordinateIsDeclared(t *testing.T) {
	dims := manifestScopeDimensions(t)

	want := specBodyCoordinateFields(t, dims)
	require.NotEmpty(t, want, "the committed contract must carry at least one body coordinate")

	got := bodyFieldsByRoute(manifestScopeRoutes(t))

	wantJSON, err := json.MarshalIndent(want, "", "  ")
	require.NoError(t, err)

	gotJSON, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)

	assert.JSONEqf(t, string(wantJSON), string(gotJSON),
		"scope.routes must declare exactly the body fields at which the contract names an organization or a ledger its "+
			"path does not carry")
}

// setBodyField writes value at field (the scope field syntax) into body, reading
// every array as a single element.
func setBodyField(body map[string]any, field, value string) {
	node := body
	segments := strings.Split(field, ".")

	for _, segment := range segments[:len(segments)-1] {
		key, isArray := strings.CutSuffix(segment, "[]")

		if !isArray {
			child, _ := node[key].(map[string]any)
			if child == nil {
				child = make(map[string]any)
				node[key] = child
			}

			node = child

			continue
		}

		elements, _ := node[key].([]any)
		if len(elements) == 0 {
			elements = []any{make(map[string]any)}
			node[key] = elements
		}

		node = elements[0].(map[string]any)
	}

	node[segments[len(segments)-1]] = value
}

// bodyScopeProbe is, per declared scope route, the request body and query string a
// probe sends and the attributes each adds to the ones the path derives: every
// declared field carries the probe value of its dimension, so each route makes
// exactly one question. The body is read for scope only for a partner credential,
// the query for every caller.
type bodyScopeProbe struct {
	body            []byte
	query           string
	bodyAttributes  map[string]string
	queryAttributes map[string]string
}

// readsBody reports whether the route reads any dimension from its body.
func (p bodyScopeProbe) readsBody() bool {
	return len(p.bodyAttributes) > 0
}

// url is the probe's request target for path.
func (p bodyScopeProbe) url(path string) string {
	if p.query == "" {
		return path
	}

	return path + "?" + p.query
}

func bodyScopeProbes(t *testing.T, dims []declaration.DeclarationDimension, values map[string]string) map[string]bodyScopeProbe {
	t.Helper()

	paramOf := make(map[string]string, len(dims))
	for _, dim := range dims {
		paramOf[dim.Name] = dim.Param
	}

	probes := make(map[string]bodyScopeProbe)

	for _, r := range manifestScopeRoutes(t) {
		body := make(map[string]any)
		query := url.Values{}
		probe := bodyScopeProbe{bodyAttributes: map[string]string{}, queryAttributes: map[string]string{}}

		for _, d := range r.Dimensions {
			value := values[paramOf[d.Name]]

			switch d.From {
			case "body":
				setBodyField(body, d.Field, value)
				probe.bodyAttributes[d.Name] = value
			case "query":
				query.Add(d.Field, value)
				probe.queryAttributes[d.Name] = value
			default:
				require.Failf(t, "unprobed carrier", "%s reads %s from %q, which the probe cannot send", r.Method+" "+r.Path, d.Name, d.From)
			}
		}

		if probe.readsBody() {
			raw, err := json.Marshal(body)
			require.NoError(t, err)

			probe.body = raw
		}

		probe.query = query.Encode()
		probes[scopeRouteKey(r)] = probe
	}

	return probes
}

// bodyReader is a request body for raw, or none when raw is empty.
func bodyReader(raw []byte) io.Reader {
	if len(raw) == 0 {
		return nil
	}

	return bytes.NewReader(raw)
}
