// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import "github.com/google/uuid"

// ScopeConfinement is, per scope dimension, the instances a scoped credential
// may see in a list. A dimension absent from it confines nothing; a dimension
// present with no ids means the list is empty, never unrestricted.
type ScopeConfinement map[string][]uuid.UUID

// IDs returns the instances the dimension is confined to, and whether it is
// confined at all.
func (s ScopeConfinement) IDs(dimension string) ([]uuid.UUID, bool) {
	ids, confined := s[dimension]

	return ids, confined
}

// ListsNothing reports whether some dimension is confined to no instance, so
// the list is empty whatever else it filters on.
func (s ScopeConfinement) ListsNothing() bool {
	for _, ids := range s {
		if len(ids) == 0 {
			return true
		}
	}

	return false
}
