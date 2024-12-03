package services

import (
	"context"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/google/uuid"
	"time"
)

// CreateLog creates an audit log for the operations of a transaction
func (uc *UseCase) CreateLog(ctx context.Context, organizationID, ledgerID, auditID uuid.UUID, data []mmodel.QueueData) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "services.create_audit_log")
	defer span.End()

	logger.Infof("Trying to create log leaves for audit ID: %v", auditID)

	var auditObj audit.Audit

	auditObj = audit.Audit{
		ID: audit.AuditID{
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			ID:             auditID.String(),
		},
		Leaves:    make(map[string]string),
		CreatedAt: time.Now(),
	}

	one, err := uc.AuditRepo.FindOne(ctx, audit.TreeCollection, auditObj.ID)
	if err != nil {
		ledgerID := auditObj.ID.LedgerID
		treeName := ledgerID[len(ledgerID)-12:]
		treeID, err := uc.TrillianRepo.CreateTree(ctx, "Tree "+treeName, ledgerID)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to create audit tree", err)

			logger.Errorf("Error creating audit tree: %v", err)

			return err
		}

		auditObj.TreeID = treeID
	} else {
		auditObj.TreeID = one.TreeID
	}

	for _, item := range data {
		logger.Infof("Saving leaf for %v", item.ID)

		leaf, err := uc.TrillianRepo.CreateLog(ctx, auditObj.TreeID, item.Value)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Error creating audit log", err)

			logger.Errorf("Error creating audit log %v", err)

			return err
		}

		auditObj.Leaves[item.ID.String()] = leaf
	}

	err = uc.AuditRepo.Create(ctx, audit.TreeCollection, &auditObj)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to save audit tree info", err)

		logger.Errorf("Error saving %s audit: %v", audit.TreeCollection, err)

		return err
	}

	return nil
}
