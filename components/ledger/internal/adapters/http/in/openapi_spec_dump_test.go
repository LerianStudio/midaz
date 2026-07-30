// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// update, when set, rewrites the committed golden spec instead of asserting
// against it: `go test ./... -run TestOpenAPISpecDump -update -buildvcs=false`.
// Without it the test compares the freshly-serialized spec byte-for-byte and
// fails on any drift.
var update = flag.Bool("update", false, "rewrite the committed OpenAPI 3.1 golden spec")

// humaSpecPath is the committed native Huma OAS 3.1 dump — the sole OpenAPI
// artifact for this component and the input the docs pipeline (generate-docs.sh /
// check-docs.sh) consumes. The legacy swaggo/openapi-generator files it once sat
// beside have been retired.
const humaSpecPath = "../../../../api/openapi.huma.yaml"

// humaSpecPathV2 is the committed native Huma OAS 3.1 dump for the SECOND,
// independent /v2 contract. The runtime serves this document separately (Servers
// ["/v2"]); the docs pipeline publishes and consolidates it alongside the v1 dump.
const humaSpecPathV2 = "../../../../api/openapi.v2.huma.yaml"

// TestOpenAPISpecDump is the golden gate for the ledger's native Huma OAS 3.1
// spec. It snapshots the shared huma.API AFTER every huma.Register (via the same
// buildUnifiedHumaAPI seam the route-diff gate uses) and serializes it offline —
// no server, DB, or Docker. huma.OpenAPI serialization is deterministic
// (top-level fields are emitted in a fixed order; every nested map — Paths,
// Components.Schemas — goes through encoding/json, which sorts map keys), so two
// runs without -update produce identical bytes and the diff is stable.
//
// With -update it rewrites the golden file; without it, it fails on any drift.
func TestOpenAPISpecDump(t *testing.T) {
	_, api := buildUnifiedHumaAPI()

	got, err := api.OpenAPI().YAML()
	require.NoError(t, err, "serialize huma OpenAPI to YAML")

	if *update {
		require.NoError(t, os.WriteFile(humaSpecPath, got, 0o644),
			"write golden spec %s", humaSpecPath)
		t.Logf("wrote golden spec %s (%d bytes)", humaSpecPath, len(got))

		return
	}

	want, err := os.ReadFile(humaSpecPath)
	require.NoErrorf(t, err, "read golden spec %s (run with -update to generate)", humaSpecPath)
	require.Equalf(t, string(want), string(got),
		"native Huma OAS 3.1 spec drifted from %s; run `go test -run TestOpenAPISpecDump -update -buildvcs=false` to regenerate",
		humaSpecPath)
}

// TestOpenAPISpecDumpV2 is the golden gate for the SECOND, independent /v2 Huma
// OAS 3.1 contract. It snapshots the v2 huma.API AFTER every huma.Register (via the
// buildUnifiedHumaAPIV2 seam that mirrors the runtime humaMountV2 mount) and
// serializes it offline, with the same deterministic-serialization and -update
// semantics as TestOpenAPISpecDump.
func TestOpenAPISpecDumpV2(t *testing.T) {
	_, api := buildUnifiedHumaAPIV2()

	got, err := api.OpenAPI().YAML()
	require.NoError(t, err, "serialize v2 huma OpenAPI to YAML")

	if *update {
		require.NoError(t, os.WriteFile(humaSpecPathV2, got, 0o644),
			"write v2 golden spec %s", humaSpecPathV2)
		t.Logf("wrote v2 golden spec %s (%d bytes)", humaSpecPathV2, len(got))

		return
	}

	want, err := os.ReadFile(humaSpecPathV2)
	require.NoErrorf(t, err, "read v2 golden spec %s (run with -update to generate)", humaSpecPathV2)
	require.Equalf(t, string(want), string(got),
		"native Huma OAS 3.1 v2 spec drifted from %s; run `go test -run TestOpenAPISpecDump -update -buildvcs=false` to regenerate",
		humaSpecPathV2)
}
