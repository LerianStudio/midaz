package services

import (
	"context"
	"encoding/json"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq/transaction"
	"github.com/LerianStudio/midaz/pkg/mlog"
)

func (uc *UseCase) CreateLog(logger mlog.Logger, transactionMessage transaction.Transaction) {

	ctx := context.TODO()

	logger.Infof("Trying to create log leaf for transaction: %v", transactionMessage.ID)

	var auditObj audit.Audit

	auditObj = audit.Audit{
		ID: audit.AuditID{
			OrganizationID: transactionMessage.OrganizationID,
			LedgerID:       transactionMessage.LedgerID,
			TransactionID:  transactionMessage.ID,
		},
		Operations: make(map[string]string),
		CreatedAt:  transactionMessage.CreatedAt,
	}

	one, err := uc.AuditRepo.FindOne(ctx, "tree", auditObj.ID) // FIXME: collection name
	if err != nil {
		tree, err := uc.CreateAuditTree(ctx, auditObj)
		if err != nil {
			logger.Error(err)
		}
		auditObj.TreeID = *tree
	} else {
		auditObj.TreeID = one.TreeID
	}

	for _, operation := range transactionMessage.Operations {
		logger.Infof("Saving log for operation %v", operation.ID)

		marshal, err := json.Marshal(operation)
		if err != nil {
			logger.Errorf("Error marshalling operation %v", err)
		}

		log, err := uc.TrillianRepo.CreateLog(ctx, auditObj.TreeID, marshal)
		if err != nil {
			logger.Errorf("Error creating log %v", err)
		}

		auditObj.Operations[operation.ID] = log
	}

	err = uc.AuditRepo.Create(ctx, "tree", &auditObj)
	if err != nil {
		logger.Error(err)
	}
}
