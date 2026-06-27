// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build testhooks

// Package bootstrap test-only hooks gated behind the `testhooks` build tag.
//
// Cross-package tests (notably internal/adapters/http/in/health_liveness_test.go,
// which lives in package `in_test`) need to reach in and reset / force the
// selfProbeOK gate. The unexported test helpers (resetSelfProbeForTest,
// markSelfProbeOKForTest) cover same-package tests, but cross-package
// callers require an exported symbol.
//
// Exporting the helpers unconditionally would link the symbols into the
// production binary — a bug is a `go tool nm` away, and the symbol names
// alone advertise the existence of a force-mark backdoor. The `testhooks`
// build tag scopes the exports to test builds only.
//
// Production builds (`go build ./cmd/app`, the Dockerfile multi-stage) build
// without the tag, so these symbols are physically absent from the resulting
// binary. The Makefile test targets (test-unit, test-integration, test-e2e)
// inject `-tags=testhooks` so cross-package tests keep compiling.
//
// Production code MUST NOT call these — once the self-probe passes it stays
// passed for the process lifetime, and forcing it via these helpers would
// short-circuit the boot-time correctness gate.
package bootstrap

// resetSelfProbeForTest forces the gate back to false. Test-only — production
// code MUST NOT call this (once self-probe passes, it stays passed for the
// process lifetime). Lives in this `testhooks`-gated file so the symbol is
// physically absent from production builds.
func resetSelfProbeForTest() {
	selfProbeOK.Store(0)
}

// markSelfProbeOKForTest forces the gate to true without running probes.
// Test-only — used by /health gating tests that don't care about the probe
// loop, only the handler behaviour.
func markSelfProbeOKForTest() {
	selfProbeOK.Store(1)
}

// ResetSelfProbeForTest is the exported test hook for callers in other
// packages (e.g. internal/adapters/http/in tests). Mirrors
// resetSelfProbeForTest but visible across packages. Production code MUST
// NOT call this — the symbol is physically absent from production builds
// because this file is only compiled under the `testhooks` build tag.
func ResetSelfProbeForTest() {
	resetSelfProbeForTest()
}

// MarkSelfProbeOKForTest is the exported test hook used by /health gating
// tests in other packages. Production code MUST NOT call this — the symbol
// is physically absent from production builds because this file is only
// compiled under the `testhooks` build tag.
func MarkSelfProbeOKForTest() {
	markSelfProbeOKForTest()
}
