// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestScopeConfinement(t *testing.T) {
	a, b := uuid.New(), uuid.New()

	t.Run("a dimension left out confines nothing", func(t *testing.T) {
		var scope ScopeConfinement

		ids, confined := scope.IDs("accountId")
		assert.False(t, confined)
		assert.Nil(t, ids)
		assert.False(t, scope.ListsNothing())
	})

	t.Run("a dimension with ids confines to them", func(t *testing.T) {
		scope := ScopeConfinement{"accountId": {a, b}}

		ids, confined := scope.IDs("accountId")
		assert.True(t, confined)
		assert.Equal(t, []uuid.UUID{a, b}, ids)
		assert.False(t, scope.ListsNothing())
	})

	t.Run("a dimension with no ids lists nothing", func(t *testing.T) {
		scope := ScopeConfinement{"accountId": {a}, "ledgerId": {}}

		ids, confined := scope.IDs("ledgerId")
		assert.True(t, confined)
		assert.Empty(t, ids)
		assert.True(t, scope.ListsNothing())
	})

	t.Run("the cursor pagination carries the confinement", func(t *testing.T) {
		header := QueryHeader{Limit: 10, Scope: ScopeConfinement{"accountId": {a}}}

		assert.Equal(t, header.Scope, header.ToCursorPagination().Scope)
		assert.Equal(t, header.Scope, header.ToOffsetPagination().Scope)
	})
}
