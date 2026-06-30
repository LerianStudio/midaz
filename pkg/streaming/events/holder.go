// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package events

import "github.com/google/uuid"

// derefString returns the pointed-to string, or "" when the pointer is nil.
// Used by the holder events because mmodel.Holder types its scalars as *string;
// a nil-safe deref keeps the constructors panic-free on a partially-built
// holder.
func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// derefUUIDString returns the canonical string form of the pointed-to UUID, or
// "" when the pointer is nil.
func derefUUIDString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}

	return id.String()
}
