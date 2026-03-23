// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"

	// GetAllMetadataOperations fetch all Operations from the repository
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) GetAllMetadataOperations(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_operations")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Retrieving operations")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(operation.Operation{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get operations on repo by metadata", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Error getting operations on repo by metadata: %v", err))

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	opFilter := operation.OperationFilter{
		OperationType: &filter.OperationType,
		Direction:     filter.Direction,
		RouteID:       filter.RouteID,
	}

	oper, cur, err := uc.OperationRepo.FindAllByAccount(ctx, organizationID, ledgerID, accountID, opFilter, filter.ToCursorPagination())
	if err != nil {
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error getting operations on repo: %v", err))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get operations on repo", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Error getting operations on repo: %v", err))

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get operations on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	var operations []*operation.Operation

	for _, o := range oper {
		if data, ok := metadataMap[o.ID]; ok {
			o.Metadata = data
			operations = append(operations, o)
		}
	}

	return operations, cur, nil
}
