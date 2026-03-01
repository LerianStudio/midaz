// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/google/uuid"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// GetAllOperationsByAccount fetch all Operations by account from the repository.
func (uc *UseCase) GetAllOperationsByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operations_by_account")
	defer span.End()

	logger.Infof("Retrieving operations by account")

	op, cur, err := uc.OperationRepo.FindAllByAccount(ctx, organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting operations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			wrappedErr := fmt.Errorf("get all operations by account: %w", pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name()))

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operations on repo", wrappedErr)

			logger.Warnf("Error getting operations on repo: %v", wrappedErr)

			return nil, libHTTP.CursorPagination{}, wrappedErr
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operations on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(op) == 0 {
		return op, cur, nil
	}

	operationIDs := make([]string, len(op))
	for i, o := range op {
		operationIDs[i] = o.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationIDs)
	if err != nil {
		wrappedErr := fmt.Errorf("get all operations by account: %w", pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name()))

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation", wrappedErr)

		logger.Warnf("Error getting metadata on mongodb operation: %v", wrappedErr)

		return nil, libHTTP.CursorPagination{}, wrappedErr
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	for i := range op {
		if data, ok := metadataMap[op[i].ID]; ok {
			op[i].Metadata = data
		}
	}

	return op, cur, nil
}
