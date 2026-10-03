//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

func TestIntegration_HolderRepo_FindAll_ConfinedToTheScope(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	organizationID := "org-scope-" + uuid.New().String()[:8]
	repo := createRepository(t, container, organizationID)
	ctx := context.Background()

	ids := make([]uuid.UUID, 0, 3)

	for i, document := range []string{"11111111101", "11111111102", "11111111103"} {
		created, err := repo.Create(ctx, organizationID, mongotestutil.CreateTestHolderSimple(t, "Scoped "+document, document))
		require.NoError(t, err, "holder %d", i)

		ids = append(ids, *created.ID)
	}

	tests := []struct {
		name  string
		scope http.ScopeConfinement
		want  []uuid.UUID
	}{
		{name: "no confinement lists every holder", want: ids},
		{name: "allowed holders", scope: http.ScopeConfinement{"holderId": {ids[0], ids[2]}}, want: []uuid.UUID{ids[0], ids[2]}},
		{name: "an empty allowed list lists nothing", scope: http.ScopeConfinement{"holderId": {}}, want: []uuid.UUID{}},
		{name: "a dimension holders cannot be confined on lists nothing", scope: http.ScopeConfinement{"ledgerId": {uuid.New()}}, want: []uuid.UUID{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			holders, err := repo.FindAll(ctx, organizationID, http.QueryHeader{Limit: 10, Page: 1, Scope: tt.scope}, false)
			require.NoError(t, err)

			got := make([]uuid.UUID, 0, len(holders))
			for _, h := range holders {
				got = append(got, *h.ID)
			}

			assert.ElementsMatch(t, tt.want, got)
		})
	}
}
