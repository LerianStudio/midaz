package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
)

// Alias parsing constants.
const (
	// aliasWithKeySeparatorParts is the number of parts when alias contains exactly one '#' separator.
	// Format: "alias#key" splits into ["alias", "key"] = 2 parts.
	aliasWithKeySeparatorParts = 2
)

func shallowCopyMetadata(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}

	return dst
}

// backfillTransactionMetadataFromBody ensures metadata is present when Mongo metadata is not yet available.
// It uses the stored transaction body as a fallback to populate transaction and operation metadata.
func backfillTransactionMetadataFromBody(tran *transaction.Transaction) {
	if tran == nil {
		return
	}

	backfillTransactionMetadata(tran)
	backfillOperationsMetadata(tran)
}

// backfillTransactionMetadata copies metadata from body to transaction if missing.
func backfillTransactionMetadata(tran *transaction.Transaction) {
	if len(tran.Metadata) == 0 && len(tran.Body.Metadata) > 0 {
		tran.Metadata = shallowCopyMetadata(tran.Body.Metadata)
	}
}

// backfillOperationsMetadata fills in missing operation metadata from transaction body.
func backfillOperationsMetadata(tran *transaction.Transaction) {
	if len(tran.Operations) == 0 {
		return
	}

	if len(tran.Body.Send.Source.From) == 0 && len(tran.Body.Send.Distribute.To) == 0 {
		return
	}

	opMetadata := buildOperationMetadataMapFromBody(tran)
	if len(opMetadata) == 0 {
		return
	}

	applyOperationMetadataFromMap(tran.Operations, opMetadata)
}

// applyOperationMetadataFromMap assigns metadata to operations that don't have it.
func applyOperationMetadataFromMap(operations []*operation.Operation, opMetadata map[string]map[string]any) {
	for i := range operations {
		if len(operations[i].Metadata) != 0 {
			continue
		}

		metadata, ok := lookupOperationMetadata(
			opMetadata,
			operations[i].Type,
			normalizeAlias(operations[i].AccountAlias),
			operations[i].BalanceKey,
			operations[i].Route,
		)
		if ok && len(metadata) != 0 {
			operations[i].Metadata = shallowCopyMetadata(metadata)
		}
	}
}

func (uc *UseCase) fetchMetadataFromOutbox(ctx context.Context, entityType, entityID string) (map[string]any, bool, error) {
	if uc == nil || uc.OutboxRepo == nil {
		return nil, false, nil
	}

	entry, err := uc.OutboxRepo.FindByEntityID(ctx, entityID, entityType)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %w", services.ErrOutboxLookup, err)
	}

	if entry == nil || len(entry.Metadata) == 0 {
		return nil, false, nil
	}

	return entry.Metadata, true, nil
}

// fetchMetadataFromOutboxBatch retrieves metadata for multiple entities in a single outbox repo call.
// It preserves the same error wrapping semantics as fetchMetadataFromOutbox (ErrOutboxLookup).
func (uc *UseCase) fetchMetadataFromOutboxBatch(ctx context.Context, entityType string, entityIDs []string) (map[string]map[string]any, map[string]error) {
	if uc == nil || uc.OutboxRepo == nil || len(entityIDs) == 0 {
		return nil, nil
	}

	metadataByID, errorsByID, err := uc.OutboxRepo.FindMetadataByEntityIDs(ctx, entityIDs, entityType)
	if err != nil {
		// Preserve per-ID logging/metrics behavior by returning an error for every requested ID.
		perID := make(map[string]error, len(entityIDs))

		wrapped := fmt.Errorf("%w: %w", services.ErrOutboxLookup, err)
		for _, id := range entityIDs {
			perID[id] = wrapped
		}

		return nil, perID
	}

	if len(errorsByID) != 0 {
		for id, e := range errorsByID {
			if e != nil {
				errorsByID[id] = fmt.Errorf("%w: %w", services.ErrOutboxLookup, e)
			}
		}
	}

	return metadataByID, errorsByID
}

func buildOperationMetadataMapFromBody(tran *transaction.Transaction) map[string]map[string]any {
	if tran == nil {
		return nil
	}

	body := tran.Body
	metadata := make(map[string]map[string]any)

	for _, from := range body.Send.Source.From {
		if len(from.Metadata) == 0 {
			continue
		}

		types := sourceOperationTypes(tran)
		for _, opType := range types {
			addOperationMetadataVariants(metadata, opType, normalizeAlias(from.AccountAlias), from.BalanceKey, from.Route, from.Metadata)
		}
	}

	for _, to := range body.Send.Distribute.To {
		if len(to.Metadata) == 0 {
			continue
		}

		addOperationMetadataVariants(metadata, constant.CREDIT, normalizeAlias(to.AccountAlias), to.BalanceKey, to.Route, to.Metadata)
	}

	return metadata
}

func operationMetadataKey(operationType, alias, balanceKey, route string) string {
	return strings.Join([]string{operationType, alias, balanceKey, route}, "|")
}

func normalizeAlias(alias string) string {
	if alias == "" {
		return alias
	}

	parts := strings.Split(alias, "#")
	switch len(parts) {
	case 1:
		return alias
	case aliasWithKeySeparatorParts:
		return parts[0]
	default:
		return parts[1]
	}
}

func lookupOperationMetadata(metadata map[string]map[string]any, operationType, alias, balanceKey, route string) (map[string]any, bool) {
	candidates := []string{
		operationMetadataKey(operationType, alias, balanceKey, route),
		operationMetadataKey(operationType, alias, balanceKey, ""),
		operationMetadataKey(operationType, alias, "", route),
		operationMetadataKey(operationType, alias, "", ""),
	}

	for _, key := range candidates {
		if data, ok := metadata[key]; ok {
			return data, true
		}
	}

	return nil, false
}

func addOperationMetadataVariants(metadata map[string]map[string]any, operationType, alias, balanceKey, route string, data map[string]any) {
	if len(data) == 0 {
		return
	}

	keys := []string{
		operationMetadataKey(operationType, alias, balanceKey, route),
		operationMetadataKey(operationType, alias, balanceKey, ""),
		operationMetadataKey(operationType, alias, "", route),
		operationMetadataKey(operationType, alias, "", ""),
	}

	for _, key := range keys {
		if _, exists := metadata[key]; !exists {
			metadata[key] = data
		}
	}
}

func sourceOperationTypes(tran *transaction.Transaction) []string {
	if tran == nil {
		return []string{constant.DEBIT}
	}

	if !tran.Body.Pending {
		return []string{constant.DEBIT}
	}

	types := []string{constant.ONHOLD}

	switch tran.Status.Code {
	case constant.APPROVED:
		types = append(types, constant.DEBIT)
	case constant.CANCELED:
		types = append(types, constant.RELEASE)
	default:
	}

	return types
}
