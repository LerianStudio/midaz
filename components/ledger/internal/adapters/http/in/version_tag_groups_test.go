// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"sort"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// opsUnder returns every non-nil *huma.Operation whose path key has the given
// prefix on the assembled contract.
func opsUnder(api huma.API, prefix string) []*huma.Operation {
	var ops []*huma.Operation

	for key, item := range api.OpenAPI().Paths {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		ops = append(ops, operationsOf(item)...)
	}

	return ops
}

// TestVersionTagGroups_OperationsRetaggedPerVersion asserts every /v1/ op carries
// only "(v1)"-suffixed resource tags and every /v2/ op only "(v2)"-suffixed ones.
func TestVersionTagGroups_OperationsRetaggedPerVersion(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()

	v1Ops := opsUnder(api, "/v1/")
	require.NotEmpty(t, v1Ops, "expected at least one /v1/ operation")

	for _, op := range v1Ops {
		require.NotEmpty(t, op.Tags, "v1 op %q must carry at least one tag", op.OperationID)

		for _, tag := range op.Tags {
			assert.Truef(t, strings.HasSuffix(tag, " (v1)"),
				"v1 op %q tag %q must be suffixed with \" (v1)\"", op.OperationID, tag)
		}
	}

	v2Ops := opsUnder(api, "/v2/")
	require.NotEmpty(t, v2Ops, "expected at least one /v2/ operation")

	for _, op := range v2Ops {
		require.NotEmpty(t, op.Tags, "v2 op %q must carry at least one tag", op.OperationID)

		for _, tag := range op.Tags {
			assert.Truef(t, strings.HasSuffix(tag, " (v2)"),
				"v2 op %q tag %q must be suffixed with \" (v2)\"", op.OperationID, tag)
		}
	}
}

// TestVersionTagGroups_RootTagsDeclaredExactlyOnce asserts the document root Tags
// list every suffixed tag exactly once, and that set equals the union of the tags
// carried by the operations.
func TestVersionTagGroups_RootTagsDeclaredExactlyOnce(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()

	seen := map[string]int{}
	for _, tag := range api.OpenAPI().Tags {
		seen[tag.Name]++
	}

	for name, count := range seen {
		assert.Equalf(t, 1, count, "root tag %q declared %d times, want exactly once", name, count)
	}

	// Every operation tag must be declared at the root.
	opTags := map[string]bool{}

	for _, op := range append(opsUnder(api, "/v1/"), opsUnder(api, "/v2/")...) {
		for _, tag := range op.Tags {
			opTags[tag] = true
		}
	}

	require.NotEmpty(t, opTags)

	for tag := range opTags {
		assert.Containsf(t, seen, tag, "operation tag %q missing from document root Tags", tag)
	}

	assert.Equal(t, len(opTags), len(seen), "root Tags must equal the set of operation tags exactly")
}

// TestVersionTagGroups_ExtensionShape asserts x-tagGroups exists with exactly two
// groups in order, V1 listing only (v1) tags and V2 only (v2) tags, each sorted.
func TestVersionTagGroups_ExtensionShape(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()

	ext := api.OpenAPI().Extensions
	require.NotNil(t, ext, "Extensions map must be initialized")

	raw, ok := ext["x-tagGroups"]
	require.True(t, ok, "x-tagGroups extension must be set")

	groups, ok := raw.([]map[string]any)
	require.Truef(t, ok, "x-tagGroups must be []map[string]any, got %T", raw)
	require.Len(t, groups, 2, "expected exactly two tag groups")

	assert.Equal(t, "V1 (deprecated)", groups[0]["name"], "first group must be the V1 (deprecated) group")
	assert.Equal(t, "V2", groups[1]["name"], "second group must be the V2 group")

	v1Tags, ok := groups[0]["tags"].([]string)
	require.Truef(t, ok, "V1 group tags must be []string, got %T", groups[0]["tags"])
	require.NotEmpty(t, v1Tags)

	for _, tag := range v1Tags {
		assert.Truef(t, strings.HasSuffix(tag, " (v1)"), "V1 group tag %q must be suffixed \" (v1)\"", tag)
	}

	assert.True(t, sort.StringsAreSorted(v1Tags), "V1 group tags must be sorted for deterministic output")

	v2Tags, ok := groups[1]["tags"].([]string)
	require.Truef(t, ok, "V2 group tags must be []string, got %T", groups[1]["tags"])
	require.NotEmpty(t, v2Tags)

	for _, tag := range v2Tags {
		assert.Truef(t, strings.HasSuffix(tag, " (v2)"), "V2 group tag %q must be suffixed \" (v2)\"", tag)
	}

	assert.True(t, sort.StringsAreSorted(v2Tags), "V2 group tags must be sorted for deterministic output")

	// Coverage: the two groups together must list EXACTLY the tag set the operations
	// carry, so no tag is rendered on an operation yet left ungrouped (hidden from the
	// Scalar sidebar), and no group lists a phantom tag no operation uses.
	grouped := map[string]bool{}
	for _, tag := range append(append([]string{}, v1Tags...), v2Tags...) {
		grouped[tag] = true
	}

	opTags := map[string]bool{}

	for _, op := range append(opsUnder(api, "/v1/"), opsUnder(api, "/v2/")...) {
		for _, tag := range op.Tags {
			opTags[tag] = true
		}
	}

	require.NotEmpty(t, opTags)
	assert.Equal(t, opTags, grouped, "x-tagGroups must cover exactly the operation tag set (no ungrouped or phantom tags)")
}

// TestVersionTagGroups_RootTagsSorted asserts the document root Tags slice is
// sorted by name — determinism guard for the committed dump.
func TestVersionTagGroups_RootTagsSorted(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()

	names := make([]string, 0, len(api.OpenAPI().Tags))
	for _, tag := range api.OpenAPI().Tags {
		names = append(names, tag.Name)
	}

	require.NotEmpty(t, names)
	assert.True(t, sort.StringsAreSorted(names), "root Tags must be sorted by name for deterministic output")
}
