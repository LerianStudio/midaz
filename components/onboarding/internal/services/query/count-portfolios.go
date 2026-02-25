// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CountPortfolios returns the number of portfolios for the specified organization and ledger.
func (uc *UseCase) CountPortfolios(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_portfolios")
	defer span.End()

	logger.Infof("Counting portfolios for organization %s and ledger %s", organizationID, ledgerID)

	count, err := uc.PortfolioRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		logger.Errorf("Error counting portfolios on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			logger.Warnf("No portfolios found for organization: %s", organizationID.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count portfolios on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count portfolios on repo", err)

		return 0, err
	}

	logger.Infof("Found %d portfolios for organization %s and ledger %s", count, organizationID, ledgerID)

	return count, nil
}
