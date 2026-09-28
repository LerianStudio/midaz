// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRecoveryCleanupMemberRoundTripIsCanonical(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	executionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	member := recoveryCleanupMember(organizationID, ledgerID, executionID)
	require.Equal(t, "11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222:33333333-3333-4333-8333-333333333333", member)

	parsedOrganizationID, parsedLedgerID, parsedExecutionID, err := parseRecoveryCleanupMember(member)
	require.NoError(t, err)
	require.Equal(t, organizationID, parsedOrganizationID)
	require.Equal(t, ledgerID, parsedLedgerID)
	require.Equal(t, executionID, parsedExecutionID)
}

func TestCleanupEngineRecoveryRejectsUnboundedOrMissingClock(t *testing.T) {
	t.Parallel()

	repository := &RedisConsumerRepository{}
	for _, test := range []struct {
		name  string
		now   time.Time
		limit int
	}{
		{name: "missing clock", limit: 1},
		{name: "zero batch", now: time.Unix(1, 0)},
		{name: "batch above repository maximum", now: time.Unix(1, 0), limit: maxRedisBatchSize + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := repository.CleanupEngineRecovery(t.Context(), test.now, test.limit)
			require.Error(t, err)
		})
	}
}

func TestParseRecoveryCleanupMemberRejectsAmbiguousIdentity(t *testing.T) {
	t.Parallel()

	for _, member := range []string{
		"",
		"11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222",
		"11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222:33333333-3333-4333-8333-333333333333:extra",
		"11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222:00000000-0000-0000-0000-000000000000",
		"11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222:33333333-3333-4333-8333-33333333333A",
	} {
		_, _, _, err := parseRecoveryCleanupMember(member)
		require.Error(t, err, member)
	}
}
