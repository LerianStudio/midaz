// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

func TestEngineScriptCompositionPreservesDependencyOrder(t *testing.T) {
	for index, fragment := range engineScriptFragments {
		require.NotEmpty(t, fragment, "engine script fragment %d must not be empty", index)
	}

	orderedSymbols := []string{
		"local function split_decimal",
		"local function decodeJSON",
		"local function isObject",
		"local cacheCompatibilityFieldNames",
		"local function redisType",
		"local function validateStoredResponse",
		"local function applyDebitPosting",
		"local function prepareExecutionProtection",
		"local function main()",
	}

	previous := -1
	for _, symbol := range orderedSymbols {
		index := strings.Index(accountingScriptRaw, symbol)
		require.Greater(t, index, previous, "%s must follow its dependencies", symbol)
		previous = index
	}

	assert.Equal(t, cachepolicy.LuaSource(accountingScriptRaw), accountingScriptSource)
	assert.Equal(t, 1, strings.Count(accountingScriptRaw, "local function main()"))
	assert.Equal(t, 1, strings.Count(accountingScriptRaw, "local ok, result = pcall(main)"))
}
