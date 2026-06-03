// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import "strings"

// IsIndexAlreadyExistsError reports whether err from index creation indicates
// the indexes already exist (a benign, idempotent outcome) rather than a real
// failure. Mongo reports this as IndexOptionsConflict or an "already exists"
// message; both are treated as success so EnsureIndexes stays idempotent.
func IsIndexAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "IndexOptionsConflict") ||
		strings.Contains(err.Error(), "already exists")
}
