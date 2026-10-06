// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
)

const (
	auditTestExchange = "test-exchange"
	auditTestKey      = "test-key"
)

var (
	auditTestOrganizationID = uuid.MustParse("0198a1b2-0000-7000-8000-000000000001")
	auditTestLedgerID       = uuid.MustParse("0198a1b2-0000-7000-8000-000000000002")
	auditTestTransactionID  = uuid.MustParse("0198a1b2-0000-7000-8000-000000000003")
)

// auditGateCases lists AUDIT_LOG_ENABLED values and whether each one enables audit publication.
var auditGateCases = []struct {
	name    string
	value   string
	enabled bool
}{
	{name: "false", value: "false", enabled: false},
	{name: "zero", value: "0", enabled: false},
	{name: "off", value: "off", enabled: false},
	{name: "no", value: "no", enabled: false},
	{name: "yes", value: "yes", enabled: false},
	{name: "one", value: "1", enabled: false},
	{name: "enabled", value: "enabled", enabled: false},
	{name: "empty", value: "", enabled: false},
	{name: "true", value: "true", enabled: true},
	{name: "upper true", value: "TRUE", enabled: true},
	{name: "title true", value: "True", enabled: true},
	{name: "padded true", value: " true ", enabled: true},
}

func TestSendLogTransactionAuditQueue(t *testing.T) {
	operations := auditTestOperations()

	t.Run("success with audit enabled", func(t *testing.T) {
		setAuditTestRoutingEnv(t)
		t.Setenv("AUDIT_LOG_ENABLED", "true")

		uc, mockRabbitMQRepo := newAuditTestUseCase(t)

		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), auditTestExchange, auditTestKey, gomock.Any()).
			Return(nil, nil).
			Times(1)

		uc.SendLogTransactionAuditQueue(context.Background(), operations, auditTestOrganizationID, auditTestLedgerID, auditTestTransactionID)
	})

	t.Run("audit disabled by default", func(t *testing.T) {
		setAuditTestRoutingEnv(t)
		unsetAuditLogEnabled(t)

		uc, mockRabbitMQRepo := newAuditTestUseCase(t)

		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		uc.SendLogTransactionAuditQueue(context.Background(), operations, auditTestOrganizationID, auditTestLedgerID, auditTestTransactionID)
	})

	t.Run("gate values", func(t *testing.T) {
		for _, tc := range auditGateCases {
			t.Run(tc.name, func(t *testing.T) {
				setAuditTestRoutingEnv(t)
				t.Setenv("AUDIT_LOG_ENABLED", tc.value)

				uc, mockRabbitMQRepo := newAuditTestUseCase(t)

				expectedCalls := 0
				if tc.enabled {
					expectedCalls = 1
				}

				mockRabbitMQRepo.EXPECT().
					ProducerDefault(gomock.Any(), auditTestExchange, auditTestKey, gomock.Any()).
					Return(nil, nil).
					Times(expectedCalls)

				uc.SendLogTransactionAuditQueue(context.Background(), operations, auditTestOrganizationID, auditTestLedgerID, auditTestTransactionID)
			})
		}
	})
}

func TestIsAuditLogEnabled(t *testing.T) {
	t.Run("unset", func(t *testing.T) {
		unsetAuditLogEnabled(t)

		assert.False(t, isAuditLogEnabled())
	})

	for _, tc := range auditGateCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AUDIT_LOG_ENABLED", tc.value)

			assert.Equal(t, tc.enabled, isAuditLogEnabled())
		})
	}
}

func TestSendLogTransactionAuditQueueAsync(t *testing.T) {
	operations := auditTestOperations()

	t.Run("audit disabled by default starts no goroutine", func(t *testing.T) {
		setAuditTestRoutingEnv(t)
		unsetAuditLogEnabled(t)

		ignoreExisting := goleak.IgnoreCurrent()

		uc, mockRabbitMQRepo := newAuditTestUseCase(t)

		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		uc.sendLogTransactionAuditQueueAsync(context.Background(), operations, auditTestOrganizationID, auditTestLedgerID, auditTestTransactionID)

		goleak.VerifyNone(t, ignoreExisting)
	})

	t.Run("audit enabled publishes once and the goroutine exits", func(t *testing.T) {
		setAuditTestRoutingEnv(t)
		t.Setenv("AUDIT_LOG_ENABLED", "true")

		ignoreExisting := goleak.IgnoreCurrent()

		uc, mockRabbitMQRepo := newAuditTestUseCase(t)

		published := make(chan struct{})

		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), auditTestExchange, auditTestKey, gomock.Any()).
			DoAndReturn(func(context.Context, string, string, []byte) (*string, error) {
				close(published)

				return nil, nil
			}).
			Times(1)

		uc.sendLogTransactionAuditQueueAsync(context.Background(), operations, auditTestOrganizationID, auditTestLedgerID, auditTestTransactionID)

		select {
		case <-published:
		case <-time.After(5 * time.Second):
			require.FailNow(t, "audit publication did not happen within the timeout")
		}

		goleak.VerifyNone(t, ignoreExisting)
	})
}

func newAuditTestUseCase(t *testing.T) (*UseCase, *rabbitmq.MockProducerRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)

	return &UseCase{RabbitMQRepo: mockRabbitMQRepo}, mockRabbitMQRepo
}

func setAuditTestRoutingEnv(t *testing.T) {
	t.Helper()

	t.Setenv("RABBITMQ_AUDIT_EXCHANGE", auditTestExchange)
	t.Setenv("RABBITMQ_AUDIT_KEY", auditTestKey)
}

// unsetAuditLogEnabled removes AUDIT_LOG_ENABLED for the test and restores its previous value on cleanup.
func unsetAuditLogEnabled(t *testing.T) {
	t.Helper()

	t.Setenv("AUDIT_LOG_ENABLED", "")
	require.NoError(t, os.Unsetenv("AUDIT_LOG_ENABLED"))
}

func auditTestOperations() []*operation.Operation {
	amountValue := decimal.NewFromInt(50)

	return []*operation.Operation{
		{
			ID:             "0198a1b2-0000-7000-8000-000000000011",
			TransactionID:  auditTestTransactionID.String(),
			OrganizationID: auditTestOrganizationID.String(),
			LedgerID:       auditTestLedgerID.String(),
			AccountID:      "0198a1b2-0000-7000-8000-000000000021",
			AccountAlias:   "alias1",
			Type:           "debit",
			AssetCode:      "USD",
			Amount: operation.Amount{
				Value: &amountValue,
			},
			Metadata: map[string]any{"key": "value"},
		},
		{
			ID:             "0198a1b2-0000-7000-8000-000000000012",
			TransactionID:  auditTestTransactionID.String(),
			OrganizationID: auditTestOrganizationID.String(),
			LedgerID:       auditTestLedgerID.String(),
			AccountID:      "0198a1b2-0000-7000-8000-000000000022",
			AccountAlias:   "alias2",
			Type:           "credit",
			AssetCode:      "EUR",
			Amount: operation.Amount{
				Value: &amountValue,
			},
			Metadata: nil,
		},
	}
}
