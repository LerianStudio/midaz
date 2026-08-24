// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// typedBodyContentType is the media type every RawBody body op files its request body
// under (the `contentType:"application/json"` tag on each request envelope's RawBody field), so
// it is the only media type the doc-only override rewrites.
const typedBodyContentType = "application/json"

// attachTypedRequestBody swaps ONLY the OpenAPI documentation schema of one body op's
// application/json request body to a $ref of T — the concrete Go type the op's handler
// decodes the raw body into. It is the generalization of publishV2CreateBodySchema
// (transaction_routes_v2.go) applied to every RawBody + SkipValidateBody op, so the
// generated contract documents the accepted fields instead of an opaque byte stream.
//
// This is DOCUMENTATION-ONLY and never touches the request path. Huma freezes decode and
// validation at huma.Register: the request validator closes over the schema captured
// during registration, and a RawBody-only op with SkipValidateBody has an empty
// inputBodyIndex, so parseBodyInto never runs — the runtime only does RawBody.SetBytes.
// Overwriting op.RequestBody.Content["application/json"].Schema AFTER registration mutates
// a DIFFERENT *Schema pointer than the captured validator schema and is never consulted at
// request time. Decode and the RFC 9457 error envelope are therefore byte-identical.
//
// The op is found by OPERATION ID over a scan of the whole document, not by path: Register
// rewrites op.Path with the version-group prefix, while the operation ID is the stable,
// globally-unique handle (it already carries the version opSuffix, so it targets the right
// version twin). If the op, its request body, or the application/json media type is not
// found, it is a defensive no-op — that must not happen for a SkipValidateBody site.
//
// Nil-guards the document so a spec-disabled build degrades to a no-op instead of panicking.
func attachTypedRequestBody[T any](api huma.API, operationID string) {
	if api == nil {
		return
	}

	oapi := api.OpenAPI()
	if oapi == nil || oapi.Components == nil || oapi.Components.Schemas == nil {
		return
	}

	t := reflect.TypeFor[T]()

	// Build the schema through the SAME registry the contract uses, so a struct input
	// resolves to a $ref of a shared component named by the ledger schema namer. This
	// registers T's whole struct graph, so stripSwaggerIgnoredFields can then resolve
	// every owning component by type.
	schema := oapi.Components.Schemas.Schema(t, true, t.Name())

	// Huma's schema generator ignores the Swaggo swaggerignore tag, so publishing the
	// typed body would leak the internal/derived fields it marks. Strip them from the
	// shared component schemas here — the same seam runs in both the production mount
	// and the offline dump, so the served spec and the committed dump agree.
	stripSwaggerIgnoredFields(oapi.Components.Schemas, t)

	for _, item := range oapi.Paths {
		for _, op := range operationsOf(item) {
			if op.OperationID != operationID || op.RequestBody == nil {
				continue
			}

			if media, ok := op.RequestBody.Content[typedBodyContentType]; ok && media != nil {
				media.Schema = schema
			}
		}
	}
}

// swaggerIgnoreTag is the Swaggo struct tag that marks a field as excluded from the
// published API contract. Huma's schema generator does NOT honor it, so the typed-body
// override must reproduce its effect on the generated component schemas.
const swaggerIgnoreTag = "swaggerignore"

// componentSchemaRefPrefix is the ref prefix the ledger/tracer schema registries are
// built with (see pkg/net/http InstallLedgerSchemaNamer -> huma.NewMapRegistry). It is
// used to reverse a component name into its registered Go type via Registry.TypeFromRef.
const componentSchemaRefPrefix = "#/components/schemas/"

// stripSwaggerIgnoredFields removes, from the shared component schemas reg holds, every
// property that maps to a struct field tagged `swaggerignore:"true"` anywhere in t's
// (nested) struct graph. swaggerignore means "not part of the API contract" for that
// field on that type everywhere, so deleting the property from the shared component the
// field's owning type resolves to is correct on every $ref that reuses it.
//
// It is DOCUMENTATION-ONLY: it mutates the generated OpenAPI component schemas, never the
// runtime decode/validation path (which Huma froze at huma.Register). The owning
// component is resolved by REVERSE type lookup over the live registry (TypeFromRef), not
// by calling Registry.Schema, so the walk never registers a new component as a side
// effect.
func stripSwaggerIgnoredFields(reg huma.Registry, t reflect.Type) {
	if reg == nil {
		return
	}

	stripFromType(reg, t, map[reflect.Type]bool{})
}

// stripFromType walks the struct graph rooted at t, deleting each swaggerignore-tagged
// field's property from its owning component. visited guards against recursive types.
func stripFromType(reg huma.Registry, t reflect.Type, visited map[reflect.Type]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}

	if t.Kind() == reflect.Map {
		stripFromType(reg, t.Elem(), visited)

		return
	}

	if t.Kind() != reflect.Struct || visited[t] {
		return
	}

	visited[t] = true

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Unexported non-embedded fields never reach the wire.
		if field.PkgPath != "" && !field.Anonymous {
			continue
		}

		jsonName := jsonFieldName(field)
		if jsonName == "-" {
			// json:"-" fields are never serialized, so there is no property to strip
			// and nothing to recurse into.
			continue
		}

		if field.Tag.Get(swaggerIgnoreTag) == "true" {
			if jsonName != "" {
				deleteComponentProperty(reg, t, jsonName)
			}

			continue
		}

		stripFromType(reg, field.Type, visited)
	}
}

// deleteComponentProperty removes property jsonName from the component schema owner
// resolves to on the registry, and from that schema's Required list. It finds the
// component by reverse type lookup so a type that was never registered (e.g. one huma
// flattened into its parent) is a silent no-op rather than a spurious new component.
func deleteComponentProperty(reg huma.Registry, owner reflect.Type, jsonName string) {
	for name, schema := range reg.Map() {
		if schema == nil || reg.TypeFromRef(componentSchemaRefPrefix+name) != owner {
			continue
		}

		delete(schema.Properties, jsonName)
		schema.Required = withoutString(schema.Required, jsonName)

		return
	}
}

// jsonFieldName returns the JSON property name a struct field serializes under: the name
// before the first comma of its json tag, the field name when the tag has no name part,
// or "-" when the field is explicitly excluded.
func jsonFieldName(field reflect.StructField) string {
	tag, ok := field.Tag.Lookup("json")
	if !ok {
		return field.Name
	}

	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return field.Name
	}

	return name
}

// withoutString returns values with every occurrence of target removed, or the input
// unchanged when target is absent.
func withoutString(values []string, target string) []string {
	out := values[:0:0]

	for _, v := range values {
		if v != target {
			out = append(out, v)
		}
	}

	return out
}
