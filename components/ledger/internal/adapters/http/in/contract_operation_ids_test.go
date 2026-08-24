// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// collectSpecOperationIDs parses the committed Huma OAS 3.1 dump at path and returns
// every operation ID it declares, keyed by operation ID and valued by the
// "METHOD path" the ID sits on. A missing or empty operationId is a finding in its own
// right, so it is reported rather than skipped.
func collectSpecOperationIDs(t *testing.T, path string) map[string][]string {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoErrorf(t, err, "Huma dump must be readable at %s", path)

	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `yaml:"operationId"`
		} `yaml:"paths"`
	}

	require.NoErrorf(t, yaml.Unmarshal(raw, &spec), "Huma dump %s must parse as YAML", path)
	require.NotEmptyf(t, spec.Paths, "Huma dump %s must declare paths", path)

	ids := make(map[string][]string)

	for pathKey, ops := range spec.Paths {
		for verb, op := range ops {
			where := strings.ToUpper(verb) + " " + pathKey

			require.NotEmptyf(t, op.OperationID,
				"%s in %s declares no operationId", where, path)

			ids[op.OperationID] = append(ids[op.OperationID], where)
		}
	}

	return ids
}

// TestContractOperationIDsAreUniqueAcrossVersions asserts every operation ID in the
// single published contract is unique — across its /v1 and /v2 operations alike, since
// one document now carries both. Client SDK generators key methods off the operation
// ID, so a collision either overwrites one method or silently drops it; the /v2 contract
// suffixes its reused resource IDs (see v2OpSuffix) precisely so they stay distinct
// from their /v1 twins here.
//
// This is NOT the primary net. The single document shares one component registry, so
// huma.AddOperation panics at boot the moment a duplicate ID is registered — the drift
// is caught long before this gate reads the dump. The gate survives for two reasons: it
// turns that boot panic into a readable, attributable failure that names the ID and both
// operations claiming it, and it is the ONLY check covering a MISSING operationId, which
// collectSpecOperationIDs flags and AddOperation does not police.
func TestContractOperationIDsAreUniqueAcrossVersions(t *testing.T) {
	t.Parallel()

	ids := collectSpecOperationIDs(t, specPath)

	var duplicated []string

	for id, where := range ids {
		if len(where) > 1 {
			sort.Strings(where)
			duplicated = append(duplicated, id+" on "+strings.Join(where, ", "))
		}
	}

	sort.Strings(duplicated)

	require.Emptyf(t, duplicated,
		"operation IDs must be unique across the /v1 and /v2 operations of %s; reused (%d):\n  %s",
		specPath, len(duplicated), strings.Join(duplicated, "\n  "))
}
