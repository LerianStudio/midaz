// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTracerOutcomeRecordRejectsParallelEconomicFact(t *testing.T) {
	t.Parallel()

	txID := uuid.New()
	record := TracerOutcomeRecord{
		Version: TracerOutcomeVersion, TransactionID: txID, OutcomeID: uuid.New(),
		OrganizationID: uuid.New(), LedgerID: uuid.New(), State: TracerOutcomeCommitted,
		Owner: "owner", EconomicPlanVersion: "1", EconomicPlanDigest: "digest",
		PreparedAtUnixMS: 1, UpdatedAtUnixMS: 2,
		EconomicOutcome: &BalanceExecutionOutcome{
			Identity: txID, Outcome: TransactionOutcomeCommitted, Owner: "owner",
			EconomicPlanVersion: "1", EconomicPlanDigest: "different",
		},
	}

	require.ErrorContains(t, record.Validate(), "differs")
	record.EconomicOutcome.EconomicPlanDigest = "digest"
	require.NoError(t, record.Validate())

	record.EconomicOutcome.Outcome = TransactionOutcomeAborted
	require.ErrorContains(t, record.Validate(), "differs")
	record.EconomicOutcome.Outcome = TransactionOutcomeCommitted
	record.EconomicOutcome.Owner = "stale-owner"
	require.ErrorContains(t, record.Validate(), "differs")
	record.EconomicOutcome.Owner = record.Owner
	record.EconomicPhase = "unknown"
	require.ErrorContains(t, record.Validate(), "economic phase")
}
