package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// =============================================================================
// Test Stubs
// =============================================================================

// balanceRepoStub provides a stub implementation of balance.Repository
// for unit testing. It allows configuring success/failure behavior.
type balanceRepoStub struct {
	createErr    error
	createCalled bool
	lastBalance  *mmodel.Balance
}

func (s *balanceRepoStub) Create(ctx context.Context, b *mmodel.Balance) error {
	s.createCalled = true
	s.lastBalance = b
	return s.createErr
}

// Implement other required interface methods as no-ops
func (s *balanceRepoStub) Find(ctx context.Context, orgID, ledgerID, id uuid.UUID) (*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) FindByAccountIDAndKey(ctx context.Context, orgID, ledgerID, accountID uuid.UUID, key string) (*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) ExistsByAccountIDAndKey(ctx context.Context, orgID, ledgerID, accountID uuid.UUID, key string) (bool, error) {
	return false, nil
}

func (s *balanceRepoStub) ListAll(ctx context.Context, orgID, ledgerID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	return nil, libHTTP.CursorPagination{}, nil
}

func (s *balanceRepoStub) ListAllByAccountID(ctx context.Context, orgID, ledgerID, accountID uuid.UUID, filter http.Pagination) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	return nil, libHTTP.CursorPagination{}, nil
}

func (s *balanceRepoStub) ListByAccountIDs(ctx context.Context, orgID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) ListByAliases(ctx context.Context, orgID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) ListByAliasesWithKeys(ctx context.Context, orgID, ledgerID uuid.UUID, aliasesWithKeys []string) ([]*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) BalancesUpdate(ctx context.Context, orgID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	return nil
}

func (s *balanceRepoStub) Update(ctx context.Context, orgID, ledgerID, id uuid.UUID, balance mmodel.UpdateBalance) (*mmodel.Balance, error) {
	return nil, nil
}

func (s *balanceRepoStub) Delete(ctx context.Context, orgID, ledgerID, id uuid.UUID) error {
	return nil
}

func (s *balanceRepoStub) DeleteAllByIDs(ctx context.Context, orgID, ledgerID uuid.UUID, ids []uuid.UUID) error {
	return nil
}

func (s *balanceRepoStub) Sync(ctx context.Context, orgID, ledgerID uuid.UUID, b mmodel.BalanceRedis) (bool, error) {
	return false, nil
}

func (s *balanceRepoStub) UpdateAllByAccountID(ctx context.Context, orgID, ledgerID, accountID uuid.UUID, balance mmodel.UpdateBalance) error {
	return nil
}

func (s *balanceRepoStub) ListByAccountID(ctx context.Context, orgID, ledgerID, accountID uuid.UUID) ([]*mmodel.Balance, error) {
	return nil, nil
}

// =============================================================================
// UNIT TESTS - handlerBalanceCreateQueue
// =============================================================================

func TestHandlerBalanceCreateQueue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	alias := "test-alias"

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		stub := &balanceRepoStub{}
		uc := &command.UseCase{
			BalanceRepo: stub,
		}
		consumer := &MultiQueueConsumer{
			UseCase: uc,
		}

		// Create valid queue message
		account := mmodel.Account{
			ID:             accountID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Name:           "Test Account",
			Type:           "deposit",
			AssetCode:      "USD",
			Alias:          &alias,
		}
		accountBytes, err := json.Marshal(account)
		require.NoError(t, err)

		queueMessage := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			QueueData: []mmodel.QueueData{
				{
					ID:    accountID,
					Value: accountBytes,
				},
			},
		}

		body, err := json.Marshal(queueMessage)
		require.NoError(t, err)

		err = consumer.handlerBalanceCreateQueue(ctx, body)

		require.NoError(t, err)
		assert.True(t, stub.createCalled, "expected Create to be called")
		assert.NotNil(t, stub.lastBalance)
		assert.Equal(t, alias, stub.lastBalance.Alias)
		assert.Equal(t, "USD", stub.lastBalance.AssetCode)
		assert.Equal(t, "deposit", stub.lastBalance.AccountType)
		assert.True(t, stub.lastBalance.AllowSending)
		assert.True(t, stub.lastBalance.AllowReceiving)
	})

	t.Run("json_unmarshal_error", func(t *testing.T) {
		t.Parallel()

		stub := &balanceRepoStub{}
		uc := &command.UseCase{
			BalanceRepo: stub,
		}
		consumer := &MultiQueueConsumer{
			UseCase: uc,
		}

		// Invalid JSON body
		invalidBody := []byte("not valid json {{{")

		err := consumer.handlerBalanceCreateQueue(ctx, invalidBody)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
		assert.False(t, stub.createCalled, "Create should not be called on unmarshal error")
	})

	t.Run("use_case_create_balance_error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("database error")
		stub := &balanceRepoStub{
			createErr: expectedErr,
		}
		uc := &command.UseCase{
			BalanceRepo: stub,
		}
		consumer := &MultiQueueConsumer{
			UseCase: uc,
		}

		// Create valid queue message
		account := mmodel.Account{
			ID:             accountID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Name:           "Test Account",
			Type:           "deposit",
			AssetCode:      "USD",
			Alias:          &alias,
		}
		accountBytes, err := json.Marshal(account)
		require.NoError(t, err)

		queueMessage := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			QueueData: []mmodel.QueueData{
				{
					ID:    accountID,
					Value: accountBytes,
				},
			},
		}

		body, err := json.Marshal(queueMessage)
		require.NoError(t, err)

		err = consumer.handlerBalanceCreateQueue(ctx, body)

		require.Error(t, err)
		assert.Equal(t, "database error", err.Error())
		assert.True(t, stub.createCalled)
	})

	t.Run("empty_queue_data", func(t *testing.T) {
		t.Parallel()

		stub := &balanceRepoStub{}
		uc := &command.UseCase{
			BalanceRepo: stub,
		}
		consumer := &MultiQueueConsumer{
			UseCase: uc,
		}

		queueMessage := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			AccountID:      accountID,
			QueueData:      []mmodel.QueueData{},
		}

		body, err := json.Marshal(queueMessage)
		require.NoError(t, err)

		err = consumer.handlerBalanceCreateQueue(ctx, body)

		assert.NoError(t, err)
		assert.False(t, stub.createCalled, "Create should not be called with empty queue data")
	})

	t.Run("invalid_account_json_in_queue_data", func(t *testing.T) {
		t.Parallel()

		stub := &balanceRepoStub{}
		uc := &command.UseCase{
			BalanceRepo: stub,
		}
		consumer := &MultiQueueConsumer{
			UseCase: uc,
		}

		// Create the queue message body directly with embedded invalid JSON
		// Since json.RawMessage validates during Marshal, we construct the body manually
		body := []byte(`{"organizationId":"` + organizationID.String() + `","ledgerId":"` + ledgerID.String() + `","accountId":"` + accountID.String() + `","queueData":[{"id":"` + accountID.String() + `","value":"not valid json"}]}`)

		err := consumer.handlerBalanceCreateQueue(ctx, body)

		require.Error(t, err)
		assert.False(t, stub.createCalled)
	})
}

// =============================================================================
// UNIT TESTS - handlerBTOQueue
// =============================================================================

func TestHandlerBTOQueue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("msgpack_unmarshal_error", func(t *testing.T) {
		t.Parallel()

		consumer := &MultiQueueConsumer{
			UseCase: &command.UseCase{},
		}

		// Invalid msgpack body (plain text that can't be unmarshaled)
		invalidBody := []byte{0xFF, 0xFE, 0xFD} // Invalid msgpack bytes

		err := consumer.handlerBTOQueue(ctx, invalidBody)

		require.Error(t, err)
	})

	t.Run("invalid_transaction_data_in_queue", func(t *testing.T) {
		t.Parallel()

		consumer := &MultiQueueConsumer{
			UseCase: &command.UseCase{},
		}

		// Create queue with invalid transaction data
		queueMessage := mmodel.Queue{
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			QueueData: []mmodel.QueueData{
				{
					ID:    uuid.New(),
					Value: []byte{}, // Empty value will fail msgpack unmarshal
				},
			},
		}

		body, err := msgpack.Marshal(queueMessage)
		require.NoError(t, err)

		err = consumer.handlerBTOQueue(ctx, body)

		// The UseCase.CreateBalanceTransactionOperationsAsync tries to
		// unmarshal QueueData.Value into TransactionProcessingPayload which fails
		require.Error(t, err)
	})
}

// =============================================================================
// UNIT TESTS - MultiQueueConsumer struct
// =============================================================================

func TestMultiQueueConsumer_StructFields(t *testing.T) {
	t.Parallel()

	uc := &command.UseCase{}
	consumer := &MultiQueueConsumer{
		UseCase: uc,
	}

	assert.NotNil(t, consumer)
	assert.Equal(t, uc, consumer.UseCase)
}
