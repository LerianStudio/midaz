//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	postgresTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// accountClosingLifecycleInstant is the closing instant of the destination account
// in the test below. A closing is compared, never measured, so the test carries no
// clock of its own.
var accountClosingLifecycleInstant = time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

// TestIntegrationAccountClosingAnswersAnInboundPendingAtItsTransition is the engine
// half of AC-08b, over the real pending lifecycle.
//
// A hold posts nothing on the destination: the pending create projects a single
// operation, the source leg, which is why a pending naming an account only as
// destination never appears in that account's participation query and never blocks
// its closing. What answers for the inbound side is this transition:
//
//   - the COMMIT credits the destination, so the closed account is part of the
//     execution and the protection refuses it before any accounting write;
//   - the CANCEL releases the source only, so the closed destination is not part of
//     the execution at all and the pending transaction still reaches a terminal
//     state.
func TestIntegrationAccountClosingAnswersAnInboundPendingAtItsTransition(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	for _, test := range []struct {
		name           string
		transition     func(*command.UseCase, context.Context, command.PendingTransitionInput) (*postgresTransaction.Transaction, error)
		terminalStatus string
		refused        bool
	}{
		{
			name:           "the commit of an inbound pending is refused by the closed destination",
			transition:     (*command.UseCase).CommitTransactionV2,
			terminalStatus: constant.APPROVED,
			refused:        true,
		},
		{
			name:           "the cancel of an inbound pending still concludes",
			transition:     (*command.UseCase).CancelTransactionV2,
			terminalStatus: constant.CANCELED,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-closing-inbound-pending")
			ctx = libObservability.ContextWithHeaderID(ctx, "request-closing-inbound-pending")

			client, _, _ := newAdapterValkey(t)
			organizationID := uuid.MustParse("b1111111-1111-4111-8111-111111111111")
			ledgerID := uuid.MustParse("b2222222-2222-4222-8222-222222222222")
			destinationAccountID := uuid.MustParse("b6666666-6666-4666-8666-666666666666")

			reader := &pendingLifecycleReader{
				client:   client,
				settings: mmodel.LedgerSettings{},
				balances: []*mmodel.Balance{
					adapterCreateBalance(organizationID, ledgerID, "b3333333-3333-4333-8333-333333333333", "b4444444-4444-4444-8444-444444444444", "@source", 100, 7),
					adapterCreateBalance(organizationID, ledgerID, "b5555555-5555-4555-8555-555555555555", destinationAccountID.String(), "@target", 20, 3),
				},
			}

			ctrl := gomock.NewController(t)
			redisRepository := txredis.NewMockRedisRepository(ctrl)
			redisRepository.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
			redisRepository.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			redisRepository.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			realAdapter, err := newAdapterWithLimits(pendingLifecycleClientProvider{client: client}, guardBootstrapLimits())
			require.NoError(t, err)

			executor := &pendingLifecycleAdapter{delegate: realAdapter}
			uc := &command.UseCase{
				TransactionRedisRepo:        redisRepository,
				TransactionReader:           reader,
				Engine:                      executor,
				AppliedTransactionCompleter: &pendingLifecycleFinalizer{outcomes: []string{constant.PENDING, test.terminalStatus}},
			}

			amount := decimal.NewFromInt(30)
			pending, replayed, err := uc.CreateTransactionV2(ctx, command.CreateTransactionV2Input{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Transaction: mtransaction.Transaction{
					Description: "inbound pending",
					Pending:     true,
					Send: mtransaction.Send{
						Asset: "USD",
						Value: amount,
						Source: mtransaction.Source{From: []mtransaction.FromTo{{
							AccountAlias: "@source",
							Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
						}}},
						Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
							AccountAlias: "@target",
							Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
						}}},
					},
				},
				TransactionStatus: constant.PENDING,
				IdempotencyTTL:    time.Minute,
			})
			require.NoError(t, err)
			require.False(t, replayed)
			require.Equal(t, constant.PENDING, pending.Status.Code)
			require.Len(t, pending.Operations, 1, "a hold projects the source leg only")
			require.Equal(t, "@source", pending.Operations[0].AccountAlias,
				"the destination of a hold owns no operation row, so it never appears in a pending query")

			// The destination closes while the pending is alive: AC-08b says nothing
			// here stops it, and the negative cache is what every later movement meets.
			markers := &engineMarkerStore{client: client}
			require.NoError(t, markers.SetAccountClosedMarker(ctx, organizationID, ledgerID, destinationAccountID, accountClosingLifecycleInstant))

			reader.persisted = pending
			reader.balances[0].Available = decimal.NewFromInt(70)
			reader.balances[0].OnHold = decimal.NewFromInt(30)
			reader.balances[0].Version = 8

			transitioned, err := test.transition(uc, ctx, command.PendingTransitionInput{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				TransactionID:  uuid.MustParse(pending.ID),
			})

			if test.refused {
				require.Error(t, err)

				var unprocessable pkg.UnprocessableOperationError
				require.ErrorAs(t, err, &unprocessable)
				require.Equal(t, constant.ErrAccountClosed.Error(), unprocessable.Code,
					"the commit is refused by the closed-account protection")

				closedAt, found, markerErr := markers.GetAccountClosedMarker(ctx, organizationID, ledgerID, destinationAccountID)
				require.NoError(t, markerErr)
				require.True(t, found)
				require.True(t, accountClosingLifecycleInstant.Equal(closedAt), "a refused commit changes no closing state")

				return
			}

			require.NoError(t, err, "the cancel of an inbound pending never reaches the closed destination")
			require.Equal(t, constant.CANCELED, transitioned.Status.Code)
			require.Equal(t, []core.PostingType{core.PostingRelease}, pendingLifecyclePostingTypes(executor.executions[1]),
				"a cancel releases the source and posts nothing on the destination")
		})
	}
}
