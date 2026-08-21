// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/stretchr/testify/assert"
)

// TestBuildDeclarationPublisher_DisabledReturnsNoStops asserts the helper is a
// no-op when RI declaration is disabled: it constructs no publisher, returns
// zero stop funcs, and never dereferences the TokenMinter (nil is passed here).
// So Run() registers no declaration runnable and boot is byte-identical to today.
//
// Epic 2.3 Task 2.3.2 adds the enabled / fail-open / drain-runnable cases; this
// file stays scoped to the disabled-path contract only.
func TestBuildDeclarationPublisher_DisabledReturnsNoStops(t *testing.T) {
	t.Parallel()

	cfg := &Config{DeclarationEnabled: false}

	stops := buildDeclarationPublisher(cfg, nil, libLog.NewNop())

	assert.Empty(t, stops, "disabled declaration must yield no stop funcs")
}
