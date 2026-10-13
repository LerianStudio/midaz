// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tenantManagerMongoMigrationName is the file name the Tenant Manager's Mongo runner applies.
var tenantManagerMongoMigrationName = regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.json$`)

// TestMetadataMongoMigrationsMatchBootList pins the tenant Mongo migrations to the single-tenant
// metadata boot list: one up/down pair per collection, creating and dropping a unique entity_id_1.
func TestMetadataMongoMigrationsMatchBootList(t *testing.T) {
	for dir, entities := range map[string][]string{
		"../../migrations/onboarding/mongodb":  onboardingMetadataEntities,
		"../../migrations/transaction/mongodb": transactionMetadataEntities,
	} {
		t.Run(dir, func(t *testing.T) {
			files, err := os.ReadDir(dir)
			require.NoError(t, err)

			want := map[string]bool{}
			for _, entity := range entities {
				want[strings.ToLower(entity)+".up"] = true
				want[strings.ToLower(entity)+".down"] = true
			}

			got := map[string]bool{}
			versionOf := map[string]string{}
			collectionOf := map[string]string{}

			for _, file := range files {
				name := file.Name()
				match := tenantManagerMongoMigrationName.FindStringSubmatch(name)
				require.NotNil(t, match, "%s does not match the Tenant Manager file name", name)

				version, direction := match[1], match[3]
				// The runner starts a fresh tenant at version 0 and applies only later versions.
				assert.NotEqual(t, strings.Repeat("0", len(version)), version, name)

				raw, err := os.ReadFile(filepath.Join(dir, name))
				require.NoError(t, err)

				var spec struct {
					Collection string `json:"collection"`
					Indexes    []struct {
						Keys    map[string]any `json:"keys"`
						Options struct {
							Name   string `json:"name"`
							Unique bool   `json:"unique"`
						} `json:"options"`
					} `json:"indexes"`
					IndexNames []string `json:"indexNames"`
				}
				require.NoError(t, json.Unmarshal(raw, &spec), name)

				if direction == "up" {
					require.Len(t, spec.Indexes, 1, name)
					assert.Equal(t, map[string]any{"entity_id": float64(1)}, spec.Indexes[0].Keys, name)
					assert.Equal(t, "entity_id_1", spec.Indexes[0].Options.Name, name)
					assert.True(t, spec.Indexes[0].Options.Unique, name)
				} else {
					assert.Equal(t, []string{"entity_id_1"}, spec.IndexNames, name)
				}

				key := spec.Collection + "." + direction
				assert.False(t, got[key], "%s is a second %s migration of %s", name, direction, spec.Collection)
				got[key] = true

				if previous, seen := versionOf[spec.Collection]; seen {
					assert.Equal(t, previous, version, "%s: up and down of %s carry different versions", name, spec.Collection)
				}

				if owner, seen := collectionOf[version]; seen {
					assert.Equal(t, owner, spec.Collection, "%s: version %s already belongs to %s", name, version, owner)
				}

				versionOf[spec.Collection], collectionOf[version] = version, spec.Collection
			}

			assert.Equal(t, want, got)
		})
	}
}
