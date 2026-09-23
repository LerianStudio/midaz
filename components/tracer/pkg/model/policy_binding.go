// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// PolicyBindingKey addresses an exact binding within the authenticated tenant's
// database. IntegrationID comes from verified service identity; ContextID comes
// from that producer's authorized context. Neither is a client-selected policy.
type PolicyBindingKey struct {
	IntegrationID string
	ContextID     string
}

// PolicyRevision is an immutable published policy reference.
type PolicyRevision struct {
	ID       uuid.UUID
	Revision int64
}

// PolicyBindingState is the revision and optimistic version of an exact binding.
// A nil state from an administration read means that no binding exists yet.
type PolicyBindingState struct {
	Policy  PolicyRevision
	Version int64
}

// BoundContextPolicy captures both the selected immutable policy and the
// binding's monotonic version. Rebinding to an older policy still advances this
// version, so an old administrator cannot overwrite an intervening activation.
type BoundContextPolicy struct {
	ContextPolicy
	BindingVersion int64
}

// Validate rejects empty or noncanonical keys instead of normalizing identities
// into a different binding. The 256-byte storage bound is not a CEL data bound.
func (k PolicyBindingKey) Validate() error {
	for _, value := range []string{k.IntegrationID, k.ContextID} {
		if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsRune(value, 0) {
			return constant.ErrInvalidRequestBody
		}
	}

	return nil
}
