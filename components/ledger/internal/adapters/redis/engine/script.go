// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	_ "embed"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

// The fragment order is the Lua dependency order. Redis still receives and
// executes the concatenated source as one atomic script.

//go:embed scripts/engine/decimal.lua
var engineDecimalScript string

//go:embed scripts/engine/json.lua
var engineJSONScript string

//go:embed scripts/engine/protocol.lua
var engineProtocolScript string

//go:embed scripts/engine/balance_cache.lua
var engineBalanceCacheScript string

//go:embed scripts/engine/request.lua
var engineRequestScript string

//go:embed scripts/engine/receipt.lua
var engineReceiptScript string

//go:embed scripts/engine/posting_algebra.lua
var enginePostingAlgebraScript string

//go:embed scripts/engine/execution.lua
var engineExecutionScript string

//go:embed scripts/engine.lua
var engineEntrypointScript string

var engineScriptFragments = [...]string{
	engineDecimalScript,
	engineJSONScript,
	engineProtocolScript,
	engineBalanceCacheScript,
	engineRequestScript,
	engineReceiptScript,
	enginePostingAlgebraScript,
	engineExecutionScript,
	engineEntrypointScript,
}

var (
	accountingScriptRaw    = strings.Join(engineScriptFragments[:], "\n")
	accountingScriptSource = cachepolicy.LuaSource(accountingScriptRaw)
	accountingScript       = redis.NewScript(accountingScriptSource)
)
