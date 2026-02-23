// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
	"go.uber.org/mock/gomock"
)

func TestSendTransactionEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("publishes approved event", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		tran := testEventTransaction(constant.APPROVED)

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		brokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-topic", tran.ID, gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc := &UseCase{BrokerRepo: brokerRepo, EventsTopic: "test-events-topic", EventsEnabled: true}
		uc.SendTransactionEvents(ctx, tran)
	})

	t.Run("does not publish when disabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		uc := &UseCase{BrokerRepo: brokerRepo, EventsTopic: "test-events-topic", EventsEnabled: false}

		uc.SendTransactionEvents(ctx, testEventTransaction(constant.APPROVED))
	})

	t.Run("does not panic when broker repo is nil", func(t *testing.T) {
		uc := &UseCase{BrokerRepo: nil, EventsTopic: "test-events-topic", EventsEnabled: true}
		uc.SendTransactionEvents(ctx, testEventTransaction(constant.APPROVED))
	})

	t.Run("publishes with canceled action key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		tran := testEventTransaction(constant.CANCELED)

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		brokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-topic", tran.ID, gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc := &UseCase{BrokerRepo: brokerRepo, EventsTopic: "test-events-topic", EventsEnabled: true}
		uc.SendTransactionEvents(ctx, tran)
	})

	t.Run("handles broker publish error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		tran := testEventTransaction(constant.APPROVED)

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		brokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-topic", tran.ID, gomock.Any()).
			Return(nil, errors.New("publish failed")).
			Times(1)

		uc := &UseCase{BrokerRepo: brokerRepo, EventsTopic: "test-events-topic", EventsEnabled: true}
		uc.SendTransactionEvents(ctx, tran)
	})

	t.Run("does not publish when transaction marshal fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		uc := &UseCase{BrokerRepo: brokerRepo, EventsTopic: "test-events-topic", EventsEnabled: true}

		badTran := *testEventTransaction(constant.APPROVED)
		badTran.Metadata = map[string]any{"invalid": func() {}}

		uc.SendTransactionEvents(ctx, &badTran)
	})

	t.Run("does not publish nil transaction", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		uc := &UseCase{BrokerRepo: brokerRepo, EventsTopic: "test-events-topic", EventsEnabled: true}

		uc.SendTransactionEvents(ctx, nil)
	})

	t.Run("falls back to status key when transaction id is empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		tran := testEventTransaction(constant.APPROVED)
		tran.ID = ""

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		brokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-events-topic", "midaz.transaction.APPROVED", gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc := &UseCase{BrokerRepo: brokerRepo, EventsTopic: "test-events-topic", EventsEnabled: true}
		uc.SendTransactionEvents(ctx, tran)
	})
}

func testEventTransaction(statusCode string) *transaction.Transaction {
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
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
}
