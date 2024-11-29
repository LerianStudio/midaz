package services

import (
	"context"
	"encoding/json"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq/transaction"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
)

// CreateLog creates an audit log for the operations of a transaction
func (uc *UseCase) CreateLog(ctx context.Context, transactionMessage transaction.Transaction) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_audit_log")
	defer span.End()

	logger.Infof("Trying to create log leaves for transaction: %v", transactionMessage.ID)

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

	one, err := uc.AuditRepo.FindOne(ctx, audit.TreeCollection, auditObj.ID)
	if err != nil {
		ledgerID := auditObj.ID.LedgerID
		treeName := ledgerID[len(ledgerID)-12:]
		treeID, err := uc.TrillianRepo.CreateTree(ctx, "Tree "+treeName, ledgerID)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to create audit tree", err)

			logger.Errorf("Error creating audit tree: %v", err)
		}

		auditObj.TreeID = treeID
	} else {
		auditObj.TreeID = one.TreeID
	}

	for _, operation := range transactionMessage.Operations {
		logger.Infof("Saving log for operation %v", operation.ID)

		marshal, err := json.Marshal(operation)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Error marshalling operation", err)

			logger.Errorf("Error marshalling operation %v", err)
		}

		log, err := uc.TrillianRepo.CreateLog(ctx, auditObj.TreeID, marshal)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Error creating audit log", err)

			logger.Errorf("Error creating audit log %v", err)
		}

		auditObj.Operations[operation.ID] = log
	}

	err = uc.AuditRepo.Create(ctx, audit.TreeCollection, &auditObj)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to save audit tree info", err)

		logger.Errorf("Error saving %s audit: %v", audit.TreeCollection, err)
	}
}
