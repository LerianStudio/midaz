// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build !windows

package wal

import "syscall"

// openNoFollowFlag returns O_NOFOLLOW on POSIX platforms so that opening
// the WAL file fails if it is a symlink (defense-in-depth against symlink
// swap attacks against the configured WAL path).
func openNoFollowFlag() int {
	return syscall.O_NOFOLLOW
}
