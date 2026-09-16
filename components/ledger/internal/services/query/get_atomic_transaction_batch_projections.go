// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

const atomicTransactionBatchProjectionLimit = 50

// GetAtomicTransactionBatchProjections rebuilds complete public transaction
// representations for recovery. PostgreSQL is read once from the primary for
// the bounded ID set; transaction and operation metadata are fetched in two
// bounded MongoDB reads. The caller restores request order from its execution
// index rather than trusting repository order.
//
//nolint:gocognit,gocyclo // one bounded read pipeline validates, enriches, and restores projection shape
func (uc *UseCase) GetAtomicTransactionBatchProjections(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	transactionIDs []uuid.UUID,
) ([]*transaction.Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if organizationID == uuid.Nil || ledgerID == uuid.Nil || len(transactionIDs) == 0 ||
		len(transactionIDs) > atomicTransactionBatchProjectionLimit {
		return nil, errors.New("atomic transaction batch projection identity is invalid")
	}

	seen := make(map[uuid.UUID]struct{}, len(transactionIDs))
	for _, transactionID := range transactionIDs {
		if transactionID == uuid.Nil {
			return nil, errors.New("atomic transaction batch projection contains a nil transaction ID")
		}

		if _, duplicate := seen[transactionID]; duplicate {
			return nil, errors.New("atomic transaction batch projection contains duplicate transaction IDs")
		}

		seen[transactionID] = struct{}{}
	}

	if uc.TransactionRepo == nil || uc.TransactionMetadataRepo == nil {
		return nil, errors.New("atomic transaction batch projection repositories are not configured")
	}

	transactions, _, err := uc.TransactionRepo.FindOrListAllWithOperations(
		readrouting.WithPrimaryRead(ctx),
		organizationID,
		ledgerID,
		append([]uuid.UUID(nil), transactionIDs...),
		pkgHTTP.Pagination{Limit: len(transactionIDs), SortOrder: "ASC"},
	)
	if err != nil {
		return nil, fmt.Errorf("read atomic transaction batch projections: %w", err)
	}

	if len(transactions) == 0 {
		return transactions, nil
	}

	transactionIDStrings := make([]string, 0, len(transactions))
	operationIDs := make([]string, 0)

	for _, tran := range transactions {
		if tran == nil {
			return nil, errors.New("atomic transaction batch projection contains a nil transaction")
		}

		transactionIDStrings = append(transactionIDStrings, tran.ID)
		sort.SliceStable(tran.Operations, func(i, j int) bool {
			if tran.Operations[i] == nil {
				return false
			}

			if tran.Operations[j] == nil {
				return true
			}

			return tran.Operations[i].ID < tran.Operations[j].ID
		})

		for _, operation := range tran.Operations {
			if operation == nil {
				return nil, errors.New("atomic transaction batch projection contains a nil operation")
			}

			operationIDs = append(operationIDs, operation.ID)
		}
	}

	transactionMetadata, err := uc.TransactionMetadataRepo.FindByEntityIDs(
		ctx,
		constant.EntityTransaction,
		transactionIDStrings,
	)
	if err != nil {
		return nil, fmt.Errorf("read atomic transaction batch metadata: %w", err)
	}

	transactionMetadataByID := make(map[string]map[string]any, len(transactionMetadata))
	for _, metadata := range transactionMetadata {
		if metadata != nil {
			transactionMetadataByID[metadata.EntityID] = metadata.Data
		}
	}

	operationMetadataByID := make(map[string]map[string]any)

	if len(operationIDs) > 0 {
		operationMetadata, err := uc.TransactionMetadataRepo.FindByEntityIDs(
			ctx,
			constant.EntityOperation,
			operationIDs,
		)
		if err != nil {
			return nil, fmt.Errorf("read atomic transaction batch operation metadata: %w", err)
		}

		operationMetadataByID = make(map[string]map[string]any, len(operationMetadata))
		for _, metadata := range operationMetadata {
			if metadata != nil {
				operationMetadataByID[metadata.EntityID] = metadata.Data
			}
		}
	}

	for _, tran := range transactions {
		if metadata, found := transactionMetadataByID[tran.ID]; found {
			tran.Metadata = metadata
		}

		source := make([]string, 0)
		destination := make([]string, 0)

		for _, operation := range tran.Operations {
			if metadata, found := operationMetadataByID[operation.ID]; found {
				operation.Metadata = metadata
			}

			switch operation.Type {
			case constant.DEBIT:
				source = append(source, operation.AccountAlias)
			case constant.CREDIT:
				destination = append(destination, operation.AccountAlias)
			case constant.BLOCK, constant.UNBLOCK:
				switch operation.Direction {
				case constant.DirectionDebit:
					source = append(source, operation.AccountAlias)
				case constant.DirectionCredit:
					destination = append(destination, operation.AccountAlias)
				}
			}
		}

		tran.Source = source
		tran.Destination = resolveDestination(destination, tran.Body)
	}

	return transactions, nil
}
