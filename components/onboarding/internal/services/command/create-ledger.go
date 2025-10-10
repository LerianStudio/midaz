// Package command implements write operations (commands) for the onboarding service.
// This file contains the command for creating a new ledger.
package command

import (
	"context"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreateLedger creates a new ledger in the repository.
//
// This use case is responsible for:
// 1. Ensuring the ledger name is unique within the organization.
// 2. Setting a default status of "ACTIVE" if none is provided.
// 3. Persisting the ledger in the PostgreSQL database.
// 4. Storing any associated metadata in MongoDB.
// 5. Returning the newly created ledger, including its metadata.
//
// Business Rules:
//   - Ledger names must be unique per organization.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization that will own the ledger.
//   - cli: The input data for creating the ledger.
//
// Returns:
//   - *mmodel.Ledger: The created ledger, complete with its metadata.
//   - error: An error if the creation fails due to a business rule violation or a database error.
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID uuid.UUID, cli *mmodel.CreateLedgerInput) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_ledger")
	defer span.End()

	logger.Infof("Trying to create ledger: %v", cli)

	var status mmodel.Status
	if cli.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cli.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cli.Status
	}

	status.Description = cli.Status.Description

	// FIXME: This logic is incorrect. FindByName returns an error if the ledger is *not* found.
	// The code should check if the error is `services.ErrDatabaseItemNotFound` and proceed in that case.
	// If the error is nil, it means a ledger with the same name already exists, and an `ErrLedgerNameConflict`
	// error should be returned. Any other error should be returned directly.
	_, err := uc.LedgerRepo.FindByName(ctx, organizationID, cli.Name)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find ledger by name", err)

		logger.Errorf("Error creating ledger: %v", err)

		return nil, err
	}

	ledger := &mmodel.Ledger{
		OrganizationID: organizationID.String(),
		Name:           cli.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	led, err := uc.LedgerRepo.Create(ctx, ledger)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger", err)

		logger.Errorf("Error creating ledger: %v", err)

		return nil, err
	}

	takeName := reflect.TypeOf(mmodel.Ledger{}).Name()

	metadata, err := uc.CreateMetadata(ctx, takeName, led.ID, cli.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create ledger metadata", err)

		logger.Errorf("Error creating ledger metadata: %v", err)

		return nil, err
	}

	led.Metadata = metadata

	return led, nil
}
