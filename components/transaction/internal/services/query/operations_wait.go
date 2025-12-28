package query

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"time"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"go.opentelemetry.io/otel/trace"
)

// ErrOperationsWaitTimeout indicates that operations polling timed out without finding results.
var ErrOperationsWaitTimeout = errors.New("operations wait timeout: async operations may still be processing")

const (
	asyncTransactionEnvVar    = "RABBITMQ_TRANSACTION_ASYNC"
	operationsWaitTimeout     = 5 * time.Second
	operationsWaitPollBackoff = 120 * time.Millisecond
)

func asyncTransactionsEnabled() bool {
	return strings.ToLower(os.Getenv(asyncTransactionEnvVar)) == "true"
}

// waitForOperations polls for operations with backoff until found or timeout.
// Return contract: returns (ops, cursor, nil) when ops found,
// (nil/empty, cursor, err) when error occurs, or
// (empty, cursor, ErrOperationsWaitTimeout) when polling times out.
func waitForOperations(ctx context.Context, fetch func(context.Context) ([]*operation.Operation, libHTTP.CursorPagination, error)) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	ops, cur, err := fetch(ctx)
	if err != nil || len(ops) > 0 || !asyncTransactionsEnabled() {
		return ops, cur, err
	}

	deadline := time.Now().Add(operationsWaitTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ops, cur, pkg.ValidateInternalError(ctx.Err(), reflect.TypeOf(operation.Operation{}).Name())
		case <-time.After(operationsWaitPollBackoff):
		}

		ops, cur, err = fetch(ctx)
		if err != nil || len(ops) > 0 {
			return ops, cur, err
		}
	}

	return ops, cur, pkg.ValidateInternalError(ErrOperationsWaitTimeout, reflect.TypeOf(operation.Operation{}).Name())
}

// extractOperationIDs extracts IDs from a slice of operations.
func extractOperationIDs(ops []*operation.Operation) []string {
	ids := make([]string, len(ops))
	for i, o := range ops {
		ids[i] = o.ID
	}

	return ids
}

// buildMetadataMap converts a slice of metadata into a map keyed by entity ID.
func buildMetadataMap(metadata []*mongodb.Metadata) map[string]map[string]any {
	metadataMap := make(map[string]map[string]any, len(metadata))
	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	return metadataMap
}

// applyMetadataToOperations applies metadata from the map to operations.
func applyMetadataToOperations(ops []*operation.Operation, metadataMap map[string]map[string]any) {
	for i := range ops {
		if data, ok := metadataMap[ops[i].ID]; ok {
			ops[i].Metadata = data
		}
	}
}

// ensureNonNilOperations returns the operations slice, or an empty slice if nil.
func ensureNonNilOperations(ops []*operation.Operation) []*operation.Operation {
	if ops == nil {
		return make([]*operation.Operation, 0)
	}

	return ops
}

// enrichOperationsWithMetadata fetches metadata for operations and attaches it.
// Returns an error if metadata fetch fails.
func (uc *UseCase) enrichOperationsWithMetadata(ctx context.Context, span *trace.Span, ops []*operation.Operation) error {
	operationIDs := extractOperationIDs(ops)

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationIDs)
	if err != nil {
		businessErr := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata on mongodb operation", businessErr)

		return businessErr
	}

	metadataMap := buildMetadataMap(metadata)
	applyMetadataToOperations(ops, metadataMap)

	return nil
}

// handleOperationsFetchResult processes the result of operations fetch.
// Handles timeout errors by returning partial results, and other errors appropriately.
func (uc *UseCase) handleOperationsFetchResult(
	logger libLog.Logger,
	span *trace.Span,
	ops []*operation.Operation,
	cur libHTTP.CursorPagination,
	err error,
) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	if errors.Is(err, ErrOperationsWaitTimeout) {
		logger.Warnf("Operations polling timed out, async operations may still be processing: %v", err)
		return ops, cur, nil
	}

	logger.Errorf("Error getting operations on repo: %v", err)

	return nil, libHTTP.CursorPagination{}, handleOperationsFetchError(span, err)
}

// handleOperationsFetchError processes errors from operations fetch and returns appropriate error.
// Returns a business error for not found cases, or an internal error otherwise.
func handleOperationsFetchError(span *trace.Span, err error) error {
	var entityNotFound *pkg.EntityNotFoundError
	if errors.As(err, &entityNotFound) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get operations on repo", err)

		return err
	}

	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get operations on repo", err)

	return pkg.ValidateInternalError(err, "Operation")
}
