// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestTracerRecoveryUsesEvidenceAndDurableAcknowledgement(t *testing.T) {
	for _, scenario := range []string{"prepared expired", "prepared dispatched", "executing approved", "executing canceled", "missing accounting", "pending accounting", "terminal replay", "unknown producer", "unknown revision", "remote failure", "invalid reply", "acknowledgement failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			store := NewMockTracerObligationStore(ctrl)
			client := NewMockContextTracerReserver(ctrl)
			evidence := NewMockTracerAccountingEvidence(ctrl)
			instant := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
			cfg := TracerRecoveryConfig{IntegrationID: "producer", Namespace: "origin-a", MaxBatch: 10, RetryInterval: time.Second, AttemptTimeout: time.Second}
			key := tracerreservation.Key{OrganizationID: uuid.MustParse("35279c72-498a-4fd5-b5b7-1bd4bd44e338"), LedgerID: uuid.MustParse("7e871c7b-24e9-4e3d-a4c2-957180a71e10"), TransactionID: uuid.MustParse("1e1dd8ae-cd4b-4cb6-a88e-2926c47906aa")}
			entry := tracerreservation.Pending{Key: key, ExecutionID: uuid.MustParse("5639dfb6-862e-4c2c-8a91-1f4f3ff54c9a"), Scope: tracercontract.ReserveScope{TenantID: "tenant-a", IntegrationID: "producer", AssetNamespace: "origin-a"}, ContractRevision: tracercontract.ReserveContractRevision, State: tracerreservation.Executing, PrepareDeadline: instant.Add(-time.Second)}
			known := true
			failed := false
			outcome := tracerreservation.Confirmed
			switch scenario {
			case "prepared expired", "prepared dispatched":
				entry.State = tracerreservation.Prepared
				known = scenario == "prepared expired"
				outcome = tracerreservation.Released
				store.EXPECT().ExpirePrepared(gomock.Any(), key, instant).Return(known, nil)
			case "terminal replay", "remote failure", "invalid reply", "acknowledgement failure":
				entry.State = tracerreservation.Confirmed
			case "unknown producer":
				entry.Scope.IntegrationID = "other"
				failed = true
				known = false
			case "unknown revision":
				entry.ContractRevision = "unsupported"
				failed = true
				known = false
			default:
				status := "APPROVED"
				if scenario == "executing canceled" {
					status = "CANCELED"
					outcome = tracerreservation.Released
				}
				if scenario == "missing accounting" {
					status = ""
					known = false
				}
				if scenario == "pending accounting" {
					status = "PENDING"
					known = false
				}
				evidence.EXPECT().ReadAccountingStatus(gomock.Any(), key).Return(status, nil)
				if known {
					store.EXPECT().SetOutcome(gomock.Any(), key, outcome, instant).Return(nil)
				}
			}
			store.EXPECT().ClaimDue(gomock.Any(), instant, instant.Add(time.Second), 10).Return([]tracerreservation.Pending{entry}, nil)
			if known {
				response := &tracercontract.TransactionCompletionResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: key.TransactionID, Status: string(outcome)}
				var remoteErr error
				if scenario == "remote failure" {
					remoteErr = errors.New("response lost")
					failed = true
				}
				if scenario == "invalid reply" {
					response.ContractRevision = ""
					failed = true
				}
				if outcome == tracerreservation.Confirmed {
					client.EXPECT().ConfirmByTransaction(gomock.Any(), key.TransactionID).Return(response, remoteErr)
				} else {
					client.EXPECT().ReleaseByTransaction(gomock.Any(), key.TransactionID).Return(response, remoteErr)
				}
				if remoteErr == nil && scenario != "invalid reply" {
					var ackErr error
					if scenario == "acknowledgement failure" {
						ackErr = errors.New("local commit unknown")
						failed = true
					}
					store.EXPECT().MarkDelivered(gomock.Any(), key, outcome, instant).Return(ackErr)
				}
			}
			processor, err := NewTracerRecoveryProcessor(store, client, evidence, cfg, func() time.Time { return instant })
			require.NoError(t, err)
			reader, factory := newReaderFactory(t)
			processor.MetricsFactory = factory
			summary, err := processor.RunOnce(tmcore.ContextWithTenantID(t.Context(), "tenant-a"))
			require.Equal(t, 1, summary.Claimed)
			expectedMetric := "recovery/unresolved"
			if known {
				expectedMetric = "confirm/delivered"
				if outcome == tracerreservation.Released {
					expectedMetric = "release/delivered"
				}
			}
			if failed {
				expectedMetric = "confirm/failed"
				if !known {
					expectedMetric = "recovery/failed"
				}
			}
			require.Equal(t, map[string]int64{expectedMetric: 1}, collectTracerCounters(t, reader))
			if failed {
				require.Error(t, err)
				require.Equal(t, 1, summary.Failed)
				require.Zero(t, summary.Delivered)
			} else {
				require.NoError(t, err)
				if known {
					require.Equal(t, 1, summary.Delivered)
				} else {
					require.Equal(t, 1, summary.Unresolved)
				}
			}
		})
	}
}
