package command

import (
	"context"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/mongodb/audit"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"time"
)

func (uc *UseCase) CreateAuditTree(ctx context.Context, organizationID, ledgerId string) (*int64, error) {

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_audit_tree")
	span.End()

	logger.Infof("Trying to create audit tree for ledger: %v", ledgerId)

	// TODO: search first to see if it already exists?

	treeID, err := uc.TrillianRepo.CreateLogTree(ctx, organizationID, ledgerId)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create audit tree", err)

		logger.Errorf("Error creating audit: %v", err)

		return nil, err
	}

	auditID := audit.AuditID{
		OrganizationID: organizationID,
		LedgerID:       ledgerId,
	}

	tree := audit.Audit{
		ID:        auditID,
		TreeID:    treeID,
		CreatedAt: time.Now(),
	}

	if err := uc.AuditRepo.Create(ctx, audit.TreeCollection, &tree); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to save audit tree info", err)

		logger.Errorf("Error saving %s audit: %v", audit.TreeCollection, err)

		return nil, err
	}

	return &treeID, nil
}
