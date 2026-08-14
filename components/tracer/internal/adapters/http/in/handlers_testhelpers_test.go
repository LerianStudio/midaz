// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

// File suffix `_test.go` keeps these helpers out of the production binary —
// the Go toolchain only compiles `*_test.go` for `go test` runs. This is the
// idiomatic equivalent of a `//go:build test` build tag (Go does not honor
// `test` as an actual tag, so the file-suffix convention is the standard
// way to scope test-only exports).
//
// H11: GetSelfProbeGateForTest used to live in handlers.go and shipped to
// every production image as a public symbol. Moving it here removes that
// leak while still allowing the external `package in_test` files (e.g.
// health_liveness_test.go) to save/restore the gate around their assertions.

// GetSelfProbeGateForTest returns the currently-installed self-probe gate so
// tests can save/restore the package-global before mutating it. Production
// code MUST NOT call this — and now it cannot, because this symbol is only
// linked into the test binary.
func GetSelfProbeGateForTest() SelfProbeGate {
	return defaultSelfProbeGate
}
