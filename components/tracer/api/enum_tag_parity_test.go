// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package api_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// =============================================================================
// Enum tag parity across the Tracer's published types.
//
// The Tracer carries TWO enum struct tags on the same fields. `enums` is the
// swag form and predates the migration; `enum` is the form Huma reads, and Huma
// is what generates the published spec now. A field with only `enums` publishes
// a bare string: the spec promises nothing, and a client discovers the accepted
// values by trial and error.
//
// Eighteen fields were given the Huma tag by copying the swag list. Nothing
// tied the two together, and the only guard over the spec is a golden dump —
// require.Equal(regenerated, committed) — which is correctness-blind by
// construction: write enum:"DAILY,BANANA", regenerate, and it passes.
//
// This walks the Tracer's Go source and requires the two tags to agree wherever
// both could apply. It fails if a Huma enum tag is dropped, if the two lists
// drift, and if a new field arrives carrying only the swag form.
// =============================================================================

// repoRoot walks up from the test's directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "reached the filesystem root without finding go.mod")

		dir = parent
	}
}

// enumMembers splits a comma-separated tag list into its members.
func enumMembers(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	return parts
}

// taggedField is one struct field carrying at least one of the two enum tags.
type taggedField struct {
	where   string
	name    string
	swag    string
	huma    string
	hasSwag bool
	hasHuma bool
}

// enumTaggedFields parses every non-test Go file under components/tracer and
// returns the struct fields carrying either enum tag.
//
// The source is parsed rather than reflected over because the tagged types are
// spread across the model, api and handler packages, and a reflection-based
// version would need a hand-maintained list of types — which is exactly the kind
// of list that silently stops covering a new field.
func enumTaggedFields(t *testing.T) []taggedField {
	t.Helper()

	root := filepath.Join(repoRoot(t), "components", "tracer")

	var found []taggedField

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()

		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}

		ast.Inspect(file, func(node ast.Node) bool {
			structType, ok := node.(*ast.StructType)
			if !ok || structType.Fields == nil {
				return true
			}

			for _, field := range structType.Fields.List {
				if field.Tag == nil {
					continue
				}

				// The literal carries its backquotes; StructTag wants them off.
				tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))

				swag, hasSwag := tag.Lookup("enums")
				humaTag, hasHuma := tag.Lookup("enum")

				if !hasSwag && !hasHuma {
					continue
				}

				name := "<embedded>"
				if len(field.Names) > 0 {
					name = field.Names[0].Name
				}

				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}

				found = append(found, taggedField{
					where:   rel + ":" + fset.Position(field.Pos()).String()[len(path)+1:],
					name:    name,
					swag:    swag,
					huma:    humaTag,
					hasSwag: hasSwag,
					hasHuma: hasHuma,
				})
			}

			return true
		})

		return nil
	})
	require.NoError(t, err, "walk the tracer source tree")

	return found
}

// TestEnumTagsAgree requires the swag and Huma enum tags to describe the same
// closed set on every field that carries either.
func TestEnumTagsAgree(t *testing.T) {
	t.Parallel()

	fields := enumTaggedFields(t)

	require.NotEmpty(t, fields,
		"no enum-tagged field was found; the walk is broken, and a broken walk passes silently")

	for _, field := range fields {
		t.Run(field.where+"/"+field.name, func(t *testing.T) {
			t.Parallel()

			require.Truef(t, field.hasHuma,
				`%s carries the swag tag enums:%q but no Huma enum tag, so the published spec declares a bare string and promises the caller nothing`,
				field.name, field.swag)

			if !field.hasSwag {
				// A Huma-only tag is fine: swag is the form being retired.
				return
			}

			require.Equalf(t, enumMembers(field.swag), enumMembers(field.huma),
				"%s declares two different closed sets: enums:%q and enum:%q. The published spec follows the Huma tag, so a drift here publishes a promise the validator does not keep",
				field.name, field.swag, field.huma)
		})
	}
}
