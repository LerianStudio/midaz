// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"reflect"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CreatePortfolio creates a new portfolio and persists it in the repository.
func (uc *UseCase) CreatePortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreatePortfolioInput) (*mmodel.Portfolio, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_portfolio")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Trying to create portfolio organizationID=%s ledgerID=%s entityID=%s", organizationID.String(), ledgerID.String(), cpi.EntityID))

	var status mmodel.Status
	if cpi.Status.IsEmpty() || libCommons.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	portfolioID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to generate portfolio ID", err)
		logger.Log(ctx, libLog.LevelError, "Error generating portfolio ID")

		return nil, err
	}

	portfolio := &mmodel.Portfolio{
		ID:             portfolioID.String(),
		EntityID:       cpi.EntityID,
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	port, err := uc.PortfolioRepo.Create(ctx, portfolio)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create portfolio", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating portfolio: %v", err))

		return nil, err
	}

	metadata, err := uc.CreateOnboardingMetadata(ctx, reflect.TypeOf(mmodel.Portfolio{}).Name(), port.ID, cpi.Metadata)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create portfolio metadata", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating portfolio metadata: %v", err))

		return nil, err
	}

	port.Metadata = metadata

	return port, nil
}
