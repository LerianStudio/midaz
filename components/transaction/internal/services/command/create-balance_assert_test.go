package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateBalance_NilAlias_Panics(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	// Create account with nil Alias
	account := mmodel.Account{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		AssetCode:      "USD",
		Type:           "deposit",
		Alias:          nil, // This should trigger assertion
	}

	accountBytes, _ := json.Marshal(account)
	queueData := mmodel.Queue{
		AccountID: uuid.New(),
		QueueData: []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: accountBytes,
			},
		},
	}

	require.Panics(t, func() {
		_ = uc.CreateBalance(ctx, queueData)
	}, "should panic when account.Alias is nil")
}
