// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/mock/gomock"
)

func TestSendLogTransactionAuditQueue(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	amountValue := decimal.NewFromInt(50)

	operations := []*operation.Operation{
		{
			ID:             uuid.New().String(),
			TransactionID:  transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			AccountAlias:   "alias1",
			Type:           "debit",
			AssetCode:      "USD",
			Amount: operation.Amount{
				Value: &amountValue,
			},
			Metadata: map[string]interface{}{"key": "value"},
		},
		{
			ID:             uuid.New().String(),
			TransactionID:  transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			AccountAlias:   "alias2",
			Type:           "credit",
			AssetCode:      "EUR",
			Amount: operation.Amount{
				Value: &amountValue,
			},
			Metadata: nil,
		},
	}

	t.Run("success with audit enabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		uc := &UseCase{BrokerRepo: brokerRepo, AuditTopic: "test-audit-topic", AuditLogEnabled: true}

		brokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-audit-topic", transactionID.String(), gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, transactionID)
	})

	t.Run("audit disabled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		uc := &UseCase{BrokerRepo: brokerRepo, AuditTopic: "test-audit-topic", AuditLogEnabled: false}

		uc.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, transactionID)
	})

	t.Run("returns early when broker repo is nil", func(t *testing.T) {
		uc := &UseCase{BrokerRepo: nil, AuditTopic: "test-audit-topic", AuditLogEnabled: true}
		uc.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, transactionID)
	})

	t.Run("handles broker publish error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		uc := &UseCase{BrokerRepo: brokerRepo, AuditTopic: "test-audit-topic", AuditLogEnabled: true}

		brokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-audit-topic", transactionID.String(), gomock.Any()).
			Return(nil, errors.New("publish failed")).
			Times(1)

		uc.SendLogTransactionAuditQueue(ctx, operations, organizationID, ledgerID, transactionID)
	})

	t.Run("returns early when operation entry is nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		uc := &UseCase{BrokerRepo: brokerRepo, AuditTopic: "test-audit-topic", AuditLogEnabled: true}

		uc.SendLogTransactionAuditQueue(ctx, []*operation.Operation{nil}, organizationID, ledgerID, transactionID)
	})

	t.Run("returns early when operation ID is invalid UUID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		brokerRepo := redpanda.NewMockProducerRepository(ctrl)
		uc := &UseCase{BrokerRepo: brokerRepo, AuditTopic: "test-audit-topic", AuditLogEnabled: true}

		broken := *operations[0]
		broken.ID = "not-a-uuid"

		uc.SendLogTransactionAuditQueue(ctx, []*operation.Operation{&broken}, organizationID, ledgerID, transactionID)
	})

}
