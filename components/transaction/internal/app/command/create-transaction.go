package command

import (
	"context"

	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
)

// CreateTransaction creates a new transaction persists data in the repository.
func (uc *UseCase) CreateTransaction(ctx context.Context, template *gold.Transaction) (*t.Transaction, error) {
	return nil, nil
}
