// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

const (
	// DefaultBalanceKey identifies the primary balance automatically created
	// for every account. It is the only balance key guaranteed to exist.
	DefaultBalanceKey = "default"

	// OverdraftBalanceKey identifies the system-managed balance that tracks
	// overdraft usage. It is auto-created when overdraft is enabled on the
	// parent balance and is protected from direct public operations.
	OverdraftBalanceKey = "overdraft"
)
