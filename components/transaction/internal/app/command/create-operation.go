package command

import (
	"context"
	"github.com/LerianStudio/midaz/common/mgrpc/account"

	gold "github.com/LerianStudio/midaz/common/gold/transaction/model"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
)

// CreateOperation creates a new operation based on transaction id and persisting data in the repository.
func (uc *UseCase) CreateOperation(ctx context.Context, accounts *account.AccountsResponse, transaction *t.Transaction, fromTo []gold.FromTo, result chan []*o.Operation, err chan error) {
	var operations []*o.Operation

	result <- operations
}
