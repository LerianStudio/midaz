package command

import (
	"context"
	goldModel "github.com/LerianStudio/midaz/pkg/gold/transaction/model"
	"github.com/google/uuid"
)

func (uc *UseCase) UpdateBalance(ctx context.Context, organizationID, ledgerID uuid.UUID, validate goldModel.Responses) error {

}