// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUnifiedContract_NoDuplicateOperationIDs asserts every operationId in the assembled
// unified document is unique. huma.OpenAPI.AddOperation panics on a duplicate id, so the
// served contract cannot even boot with two ops sharing one; this pins the invariant at the
// document level — across every v1 and v2 family at once — catching a v2 twin that dropped
// its version suffix. The invariant is document-global, so one test covers the whole surface
// rather than one copy per family.
func TestUnifiedContract_NoDuplicateOperationIDs(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()

	seen := make(map[string]string)

	for key, item := range api.OpenAPI().Paths {
		for _, operation := range operationsOf(item) {
			where := operation.Method + " " + key

			prior, dup := seen[operation.OperationID]
			require.Falsef(t, dup, "operationId %q is published twice: %s and %s",
				operation.OperationID, prior, where)

			seen[operation.OperationID] = where
		}
	}
}
