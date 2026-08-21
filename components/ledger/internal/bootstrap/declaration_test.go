// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/stretchr/testify/assert"
)

// stubTokenMinter is a no-op declaration.TokenMinter used where a non-nil minter
// is required but must never be dereferenced (the disabled-flag path returns
// before any publisher is constructed). The full wiring suite (two real slugs,
// fail-open under an httptest 5xx, no-goroutine-leak) is Task 1.4.2.
type stubTokenMinter struct{}

func (stubTokenMinter) GetApplicationToken(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

// TestBuildDeclarationPublishers_DisabledReturnsNoStops asserts the helper is a
// no-op when RI declaration is disabled: it constructs no publisher and returns
// zero stop funcs, so launcherApps registers no runnable. Not parallel: this
// package runs goleak.VerifyTestMain (see goleak_test.go).
func TestBuildDeclarationPublishers_DisabledReturnsNoStops(t *testing.T) {
	cfg := &Config{DeclarationEnabled: false}

	stops := buildDeclarationPublishers(cfg, nil, libLog.NewNop())

	assert.Empty(t, stops, "disabled declaration must yield no stop funcs")
}
