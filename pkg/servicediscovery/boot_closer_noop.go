// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build !libsd

package servicediscovery

import (
	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

// BootCloser is the no-op boot-failure closer for the DEFAULT build. With
// discovery disabled no boot-time Resolve watcher is spawned, so there is
// nothing to tear down. The real closer lives in boot_closer_libsd.go under
// //go:build libsd (see the package doc and TODO(3482)).
type BootCloser struct{}

// NewBootCloser returns a no-op BootCloser. It accepts the same arguments as the
// real constructor so the composition roots wire it identically regardless of
// build.
func NewBootCloser(_ libLog.Logger, _ *Manager) *BootCloser {
	return &BootCloser{}
}

// Disarm is a no-op. Nil-safe on the receiver.
func (b *BootCloser) Disarm() {}

// CloseOnBootFailure is a no-op: the default build spawns no boot-time watcher
// to tear down. Nil-safe on the receiver.
func (b *BootCloser) CloseOnBootFailure() {}
