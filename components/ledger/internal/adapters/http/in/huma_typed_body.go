// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

// typedBodyContentType is the media type every RawBody body op files its request body
// under (the `contentType:"application/json"` tag on each *InputHuma.RawBody field), so
// it is the only media type the doc-only override rewrites.
const typedBodyContentType = "application/json"

// attachTypedRequestBody swaps ONLY the OpenAPI documentation schema of one body op's
// application/json request body to a $ref of T — the concrete Go type the op's handler
// decodes the raw body into. It is the generalization of publishV2CreateBodySchema
// (transaction_v2_register.go) applied to every RawBody + SkipValidateBody op, so the
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
	// resolves to a $ref of a shared component named by the ledger schema namer.
	schema := oapi.Components.Schemas.Schema(t, true, t.Name())

	for _, item := range oapi.Paths {
		if item == nil {
			continue
		}

		for _, op := range []*huma.Operation{
			item.Get, item.Put, item.Post, item.Delete,
			item.Options, item.Head, item.Patch, item.Trace,
		} {
			if op == nil || op.OperationID != operationID || op.RequestBody == nil {
				continue
			}

			if media, ok := op.RequestBody.Content[typedBodyContentType]; ok && media != nil {
				media.Schema = schema
			}
		}
	}
}
