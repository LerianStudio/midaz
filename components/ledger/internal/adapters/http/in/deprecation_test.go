// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
)

// operationsForPathItem returns the non-nil operations on a huma.PathItem paired
// with their HTTP verb, so a caller can assert per-operation metadata without
// repeating the eight-method fan-out.
func operationsForPathItem(item *huma.PathItem) map[string]*huma.Operation {
	verbs := map[string]*huma.Operation{
		"GET":     item.Get,
		"PUT":     item.Put,
		"POST":    item.Post,
		"DELETE":  item.Delete,
		"OPTIONS": item.Options,
		"HEAD":    item.Head,
		"PATCH":   item.Patch,
		"TRACE":   item.Trace,
	}

	present := make(map[string]*huma.Operation, len(verbs))

	for verb, op := range verbs {
		if op != nil {
			present[verb] = op
		}
	}

	return present
}

// TestV1OperationsAreDeprecated locks the version-deprecation contract on the
// assembled Huma document: EVERY /v1 operation must carry Deprecated=true and NO
// /v2 operation may be deprecated. It builds the same unified contract the spec
// dump serializes, so the served spec and the committed dump cannot diverge.
func TestV1OperationsAreDeprecated(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	doc := api.OpenAPI()

	var v1Ops, v2Ops int

	for key, item := range doc.Paths {
		switch {
		case strings.HasPrefix(key, "/v1/"):
			for verb, op := range operationsForPathItem(item) {
				v1Ops++

				if !op.Deprecated {
					t.Errorf("v1 operation %s %s must be Deprecated=true", verb, key)
				}
			}
		case strings.HasPrefix(key, "/v2/"):
			for verb, op := range operationsForPathItem(item) {
				v2Ops++

				if op.Deprecated {
					t.Errorf("v2 operation %s %s must NOT be deprecated", verb, key)
				}
			}
		}
	}

	if v1Ops == 0 {
		t.Fatal("expected at least one /v1 operation in the assembled contract")
	}

	if v2Ops == 0 {
		t.Fatal("expected at least one /v2 operation in the assembled contract")
	}
}
