package services

import (
	"context"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
)

func (uc *UseCase) CreateAuditTree(ctx context.Context, auditObj audit.Audit) (*int64, error) {

	// TODO: move to create-log

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_audit_tree")
	span.End()

	logger.Infof("Trying to create audit tree for ledger: %v", auditObj.ID.LedgerID)

	treeID, err := uc.TrillianRepo.CreateLogTree(ctx, "My LOG Tree", auditObj.ID.LedgerID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create audit tree", err)

		logger.Errorf("Error creating audit: %v", err)

		return nil, err
	}

	logger.Infof("Created audit tree for ledger: %v", treeID)

	auditObj.TreeID = treeID

	if err := uc.AuditRepo.Create(ctx, audit.TreeCollection, &auditObj); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to save audit tree info", err)

		logger.Errorf("Error saving %s audit: %v", audit.TreeCollection, err)

		return nil, err
	}

	return &treeID, nil
}
