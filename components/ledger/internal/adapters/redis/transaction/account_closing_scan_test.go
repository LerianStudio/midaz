// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestParseAccountProtectionScope pins that a discovered key resolves to the
// complete scope it was built from, with or without a tenant prefix in front. The
// scope is what confines everything the reconciliation then does, so a key must
// never resolve to an account alone.
func TestParseAccountProtectionScope(t *testing.T) {
	organizationID := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000002")
	accountID := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000003")

	want := AccountProtectionScope{OrganizationID: organizationID, LedgerID: ledgerID, AccountID: accountID}

	keys := []string{
		utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID),
		utils.AccountAdminOwnershipKey(organizationID, ledgerID, accountID),
		"tenant-42:" + utils.AccountClosingMarkerKey(organizationID, ledgerID, accountID),
	}

	for _, key := range keys {
		scope, err := parseAccountProtectionScope(key)

		require.NoErrorf(t, err, "key %q", key)
		assert.Equal(t, want, scope)
	}
}

// TestParseAccountProtectionScopeRejectsAnUnresolvableKey pins that a key which
// does not carry three identifiers is reported instead of resolving to a partial
// scope, which is how a reconciliation could otherwise reach another tenant's
// account.
func TestParseAccountProtectionScopeRejectsAnUnresolvableKey(t *testing.T) {
	keys := []string{
		"account-closing:{transactions}",
		"account-closing:{transactions}:not-a-uuid:aaaaaaaa-0000-0000-0000-000000000002:aaaaaaaa-0000-0000-0000-000000000003",
		"account-closing:{transactions}:aaaaaaaa-0000-0000-0000-000000000001:not-a-uuid:aaaaaaaa-0000-0000-0000-000000000003",
		"account-closing:{transactions}:aaaaaaaa-0000-0000-0000-000000000001:aaaaaaaa-0000-0000-0000-000000000002:not-a-uuid",
	}

	for _, key := range keys {
		_, err := parseAccountProtectionScope(key)

		require.Errorf(t, err, "key %q", key)
	}
}
