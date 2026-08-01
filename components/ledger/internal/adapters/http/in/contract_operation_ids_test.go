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

// TestContractOperationIDsAreUniqueAcrossVersions pins the constraint the published hub
// spec imposes on the per-version contracts it joins. The joiner makes PATH keys unique
// by prefixing each document with its version segment; it does not touch operation IDs.
// Two operations sharing an ID therefore survive into the joined document, where client
// SDK generators key methods off the operation ID and either collide or silently drop
// one of them.
//
// The gate is over the COMMITTED dumps, which are the joiner's actual inputs; the golden
// gates (TestOpenAPISpecDump / TestOpenAPISpecDumpV2) are what tie those dumps back to
// the registrations. It asserts uniqueness within each dump and disjointness between
// them, naming every offending ID and the operations that claim it.
func TestContractOperationIDsAreUniqueAcrossVersions(t *testing.T) {
	t.Parallel()

	v1 := collectSpecOperationIDs(t, specPath)
	v2 := collectSpecOperationIDs(t, specPathV2)

	for _, dump := range []struct {
		path string
		ids  map[string][]string
	}{{specPath, v1}, {specPathV2, v2}} {
		var duplicated []string

		for id, where := range dump.ids {
			if len(where) > 1 {
				sort.Strings(where)
				duplicated = append(duplicated, id+" on "+strings.Join(where, ", "))
			}
		}

		sort.Strings(duplicated)

		require.Emptyf(t, duplicated,
			"operation IDs must be unique within %s; reused (%d):\n  %s",
			dump.path, len(duplicated), strings.Join(duplicated, "\n  "))
	}

	var shared []string

	for id, whereV1 := range v1 {
		whereV2, ok := v2[id]
		if !ok {
			continue
		}

		shared = append(shared, id+" on "+strings.Join(whereV1, ", ")+" and "+strings.Join(whereV2, ", "))
	}

	sort.Strings(shared)

	require.Emptyf(t, shared,
		"operation IDs must not repeat across the v1 and v2 contracts — the hub join keeps both;\n"+
			"suffix the newer contract's IDs (see crmOpSuffixV1). Shared (%d):\n  %s",
		len(shared), strings.Join(shared, "\n  "))
}
