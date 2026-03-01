// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)

func TestSendDecisionLifecycleEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("publishes decision lifecycle event", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		tran := testDecisionEventTransaction()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		brokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-topic", tran.ID, gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc := &UseCase{BrokerRepo: brokerRepo, EventsTopic: "test-events-topic", EventsEnabled: true}
		uc.SendDecisionLifecycleEvent(ctx, tran, pkgTransaction.DecisionContract{}, pkgTransaction.DecisionLifecycleActionAuthorizationRequested)
	})

	t.Run("does not publish when disabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		uc := &UseCase{BrokerRepo: brokerRepo, EventsTopic: "test-events-topic", EventsEnabled: false}

		uc.SendDecisionLifecycleEvent(ctx, testDecisionEventTransaction(), pkgTransaction.DecisionContract{}, pkgTransaction.DecisionLifecycleActionAuthorizationRequested)
	})

	t.Run("normalizes empty decision contract fields", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		tran := testDecisionEventTransaction()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		brokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-topic", tran.ID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, payload []byte) (*string, error) {
				event := mmodel.Event{}
				require.NoError(t, json.Unmarshal(payload, &event))

				decisionPayload := pkgTransaction.DecisionLifecycleEvent{}
				require.NoError(t, json.Unmarshal(event.Payload, &decisionPayload))

				assert.Equal(t, pkgTransaction.DecisionEndpointModeSync, decisionPayload.DecisionContract.EndpointMode)
				assert.Equal(t, pkgTransaction.GuaranteeModelSourceDurable, decisionPayload.DecisionContract.GuaranteeModel)
				assert.Equal(t, pkgTransaction.DestinationBlockedPolicyCreditAnyway, decisionPayload.DecisionContract.DestinationBlockedPolicy)
				assert.Equal(t, int64(150), decisionPayload.DecisionContract.DecisionLatencySLOMs)

				return nil, nil
			}).
			Times(1)

		uc := &UseCase{BrokerRepo: brokerRepo, EventsTopic: "test-events-topic", EventsEnabled: true}
		uc.SendDecisionLifecycleEvent(ctx, tran, pkgTransaction.DecisionContract{}, pkgTransaction.DecisionLifecycleActionAuthorizationRequested)
	})

	t.Run("uses dedicated decision events topic when configured", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		tran := testDecisionEventTransaction()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		brokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-decision-topic", tran.ID, gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc := &UseCase{
			BrokerRepo:          brokerRepo,
			EventsTopic:         "test-events-topic",
			DecisionEventsTopic: "test-decision-topic",
			EventsEnabled:       true,
		}

		uc.SendDecisionLifecycleEvent(ctx, tran, pkgTransaction.DecisionContract{}, pkgTransaction.DecisionLifecycleActionAuthorizationRequested)
	})

	t.Run("falls back to events topic when dedicated decision topic is blank", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		tran := testDecisionEventTransaction()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		brokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-topic", tran.ID, gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc := &UseCase{
			BrokerRepo:          brokerRepo,
			EventsTopic:         "test-events-topic",
			DecisionEventsTopic: "   ",
			EventsEnabled:       true,
		}

		uc.SendDecisionLifecycleEvent(ctx, tran, pkgTransaction.DecisionContract{}, pkgTransaction.DecisionLifecycleActionAuthorizationRequested)
	})
}

func testDecisionEventTransaction() *transaction.Transaction {
	const statusCode = "APPROVED"

	description := statusCode
	status := transaction.Status{Code: statusCode, Description: &description}
	amount := decimal.NewFromInt(100)
	parentTransactionID := libCommons.GenerateUUIDv7().String()

	return &transaction.Transaction{
		ID:                       libCommons.GenerateUUIDv7().String(),
		ParentTransactionID:      &parentTransactionID,
		OrganizationID:           libCommons.GenerateUUIDv7().String(),
		LedgerID:                 libCommons.GenerateUUIDv7().String(),
		Description:              description,
		Status:                   status,
		Amount:                   &amount,
		AssetCode:                "BRL",
		ChartOfAccountsGroupName: "ChartOfAccountsGroupName",
	}
}
