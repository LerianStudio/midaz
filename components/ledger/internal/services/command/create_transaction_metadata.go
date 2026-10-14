// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
)

// CreateTransactionMetadata persists the given metadata in the transaction-side MongoDB
// metadata store for the specified entity. It mirrors CreateOnboardingMetadata over
// TransactionMetadataRepo. Nil or empty metadata persists no document and returns (nil, nil).
//
// Creates call it before the SQL insert, with the ID the use case pre-assigned to the
// entity, so a metadata store outage fails the request before anything becomes durable.
// A write failure is recorded on this helper's span by error class and returned without a log
// line: the calling use case owns the single log of the error.
func (uc *UseCase) CreateTransactionMetadata(ctx context.Context, entityName, entityID string, metadata map[string]any) (map[string]any, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_transaction_metadata")
	defer span.End()

	if len(metadata) == 0 {
		return nil, nil
	}

	now := time.Now()

	meta := mongodb.Metadata{
		EntityID:   entityID,
		EntityName: entityName,
		Data:       metadata,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := uc.TransactionMetadataRepo.Create(ctx, entityName, &meta); err != nil {
		recordCommandSpanError(span, "Failed to create metadata", err)

		return nil, err
	}

	return metadata, nil
}
