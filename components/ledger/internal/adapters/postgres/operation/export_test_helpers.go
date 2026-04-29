// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

// ExportedOperationColumnListLen returns the length of the canonical
// operationColumnList for use by cross-package tests that need to verify
// column-list parity (e.g., transaction package's prefixed copy).
func ExportedOperationColumnListLen() int {
	return len(operationColumnList)
}
