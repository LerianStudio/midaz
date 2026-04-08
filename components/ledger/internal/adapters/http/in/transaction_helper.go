// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// transactionPathParams holds the IDs extracted from URL path parameters.
// TransactionID is uuid.Nil when the route has no :transaction_id segment.
type transactionPathParams struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	TransactionID  uuid.UUID
}

// readPathParams extracts organization, ledger, and (optional) transaction
// IDs from Fiber locals populated by the UUID-parsing middleware.
func readPathParams(c *fiber.Ctx) (*transactionPathParams, error) {
	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return nil, err
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return nil, err
	}

	transactionID := uuid.Nil
	if c.Locals("transaction_id") != nil {
		transactionID, err = http.GetUUIDFromLocals(c, "transaction_id")
		if err != nil {
			return nil, err
		}
	}

	return &transactionPathParams{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		TransactionID:  transactionID,
	}, nil
}

// generateTransactionID creates a new UUIDv7 for a transaction.
func generateTransactionID(ctx context.Context, logger libLog.Logger, span trace.Span) (uuid.UUID, error) {
	transactionID, err := libCommons.GenerateUUIDv7()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to generate transaction id", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to generate transaction id: %v", err))

		return uuid.Nil, pkg.InternalServerError{
			Code:    "INTERNAL_SERVER_ERROR",
			Title:   "Internal Server Error",
			Message: "Failed to generate transaction id",
			Err:     err,
		}
	}

	return transactionID, nil
}

// buildParentTransactionID converts a parent UUID to a string pointer,
// returning nil when the parent is uuid.Nil (no parent).
func buildParentTransactionID(parentID uuid.UUID) *string {
	if parentID == uuid.Nil {
		return nil
	}

	s := parentID.String()

	return &s
}

// getAliasWithoutKey strips the "#key" suffix from alias strings,
// returning only the alias portion before the first "#".
func getAliasWithoutKey(array []string) []string {
	result := make([]string, len(array))

	for i, str := range array {
		parts := strings.Split(str, "#")
		result[i] = parts[0]
	}

	return result
}
