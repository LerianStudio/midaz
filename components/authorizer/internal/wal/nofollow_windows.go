// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build windows

package wal

// openNoFollowFlag is a no-op on Windows because NTFS symlinks are handled
// by the reparse-point subsystem rather than the O_NOFOLLOW open flag, which
// Windows does not implement. Returning 0 keeps the flag bitmask safe.
func openNoFollowFlag() int {
	return 0
}
