package query

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
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

func waitForOperations(ctx context.Context, fetch func(context.Context) ([]*operation.Operation, libHTTP.CursorPagination, error)) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	ops, cur, err := fetch(ctx)
	if err != nil || len(ops) > 0 || !asyncTransactionsEnabled() {
		return ops, cur, err
	}

	deadline := time.Now().Add(operationsWaitTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ops, cur, ctx.Err()
		case <-time.After(operationsWaitPollBackoff):
		}

		ops, cur, err = fetch(ctx)
		if err != nil || len(ops) > 0 {
			return ops, cur, err
		}
	}

	return ops, cur, ErrOperationsWaitTimeout
}
