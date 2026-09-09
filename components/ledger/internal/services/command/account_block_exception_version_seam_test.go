// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The account-block exception is a /v2 contract, and like the fee and tracer
// seams the guarantee is structural rather than a runtime flag: a /v1 pipeline
// NAMES the resolver nowhere, so a /v1 request cannot acquire a grant, a grant
// rejection, or a consumed identifier from a version upgrade — whatever a caller
// manages to put on the input struct.
//
// A runtime test cannot prove that. It can only show that one call with a nil
// identifier behaves; it cannot show that no code path reads a non-nil one. This
// reads the source instead, the same way the fee seam's gates do.

// grantResolverName is the single seam every pipeline that honors an exception has
// to go through. Nothing else reads the cached grant.
const grantResolverName = "resolveAccountBlockExceptionGrant"

// readPipelineSource returns the source of one command file, so a gate can assert
// over what the pipeline actually names.
func readPipelineSource(t *testing.T, path string) string {
	t.Helper()

	src, err := os.ReadFile(path)
	require.NoErrorf(t, err, "read %s", path)

	return string(src)
}

// funcBody returns the body of the named top-level function, from its signature
// to the next top-level declaration. Crude on purpose: it needs only to separate
// the version twins that sit in one file.
func funcBody(t *testing.T, src, funcName string) string {
	t.Helper()

	marker := ") " + funcName + "("
	start := strings.Index(src, marker)
	require.NotEqualf(t, -1, start, "function %s must exist", funcName)

	rest := src[start:]
	if end := strings.Index(rest, "\nfunc "); end != -1 {
		return rest[:end]
	}

	return rest
}

// TestAccountBlockExceptionSeam_OnlyV2PipelinesResolveTheGrant is the version
// gate. Each pair below lives in ONE file, so the assertion is about which twin
// names the resolver, not about which file it appears in.
func TestAccountBlockExceptionSeam_OnlyV2PipelinesResolveTheGrant(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		path      string
		honoring  string
		abstinent string
	}{
		{
			name:      "create",
			path:      "create_transaction_v2.go",
			honoring:  "CreateTransactionV2",
			abstinent: "",
		},
		{
			name:      "revert",
			path:      "revert_transaction.go",
			honoring:  "createRevertV2",
			abstinent: "createRevertV1",
		},
		{
			name:      "pending transition",
			path:      "commit_transaction.go",
			honoring:  "transitionPendingV2",
			abstinent: "transitionPendingV1",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := readPipelineSource(t, tt.path)

			assert.Containsf(t, funcBody(t, src, tt.honoring), grantResolverName,
				"%s honors the /v2 exception contract, so it must name the resolver", tt.honoring)

			if tt.abstinent == "" {
				return
			}

			assert.NotContainsf(t, funcBody(t, src, tt.abstinent), grantResolverName,
				"%s is frozen at what /v1 shipped with: naming the resolver would let a /v1 "+
					"request acquire a grant rejection it never asked for", tt.abstinent)
		})
	}
}

// TestAccountBlockExceptionSeam_V1CreateNamesNoResolver covers the create twin,
// which lives in its own file rather than beside its sibling.
func TestAccountBlockExceptionSeam_V1CreateNamesNoResolver(t *testing.T) {
	t.Parallel()

	src := readPipelineSource(t, "create_transaction_v1.go")

	assert.NotContains(t, src, grantResolverName,
		"the /v1 create pipeline must name the exception resolver nowhere")
	assert.NotContains(t, src, "AccountBlockExceptionID",
		"the /v1 create input must not carry the identifier at all")
}

// TestAccountBlockExceptionSeam_GrantReachesTheScriptThroughOneSeam proves the
// resolved grant is handed to the balance step and nothing else does the handing:
// exactly the three /v2 pipelines populate the field, and the shared balance step
// reads it.
//
// Without this, a fourth call site could pass a grant assembled some other way and
// bypass the single read the version gate above guards.
func TestAccountBlockExceptionSeam_GrantReachesTheScriptThroughOneSeam(t *testing.T) {
	t.Parallel()

	const grantField = "AccountBlockExceptionGrant:"

	populating := map[string]string{
		"create":             "create_transaction_v2.go",
		"revert":             "revert_transaction.go",
		"pending transition": "transition_pending_steps.go",
	}

	for name, path := range populating {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			src := readPipelineSource(t, path)

			assert.Containsf(t, src, grantField,
				"%s must hand the resolved grant to the balance step", name)
			assert.Equalf(t, 1, strings.Count(src, grantField),
				"%s must populate the grant exactly once", name)
		})
	}

	t.Run("the balance step forwards it to the script", func(t *testing.T) {
		t.Parallel()

		src := readPipelineSource(t, "process_balance_operations.go")

		assert.Contains(t, src, "input.AccountBlockExceptionGrant",
			"the balance step must forward the grant to the atomic script")
	})
}
