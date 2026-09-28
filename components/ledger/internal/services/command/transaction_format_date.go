// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// formatTransactionDate validates the transactionDate field and returns the
// effective timestamp to use for CreatedAt. Rules:
//
//   - If transactionDate is nil or zero, the current time is returned (server-assigned).
//   - If transactionDate is in the future, it is rejected (error 0121).
//   - If the transaction is pending, a custom transactionDate is rejected (error 0122)
//     because pending transactions are committed later with their own timestamp.
func formatTransactionDate(ctx context.Context, span trace.Span, transactionInput mtransaction.Transaction, transactionStatus string) (time.Time, error) {
	now := time.Now()
	logger := libObservability.NewLoggerFromContext(ctx)

	transactionDate, err := resolveTransactionDateAt(transactionInput, transactionStatus, now)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction date validation failed", err)

		message := "Pending transaction cannot have a custom transaction date"
		if transactionInput.TransactionDate != nil && transactionInput.TransactionDate.After(now) {
			message = "Transaction date cannot be a future date"
		}

		logger.Log(ctx, libLog.LevelWarn, message, libLog.Err(err))

		return time.Time{}, err
	}

	return transactionDate, nil
}

// resolveTransactionDateAt applies the transaction-date rules against a caller-
// supplied instant. The singular command supplies time.Now; the atomic batch
// supplies its injected clock so every replay-sensitive timestamp is testable.
func resolveTransactionDateAt(transactionInput mtransaction.Transaction, transactionStatus string, now time.Time) (time.Time, error) {
	if transactionInput.TransactionDate == nil || transactionInput.TransactionDate.IsZero() {
		return now, nil
	}

	if transactionInput.TransactionDate.After(now) {
		return time.Time{}, pkg.ValidateBusinessError(constant.ErrInvalidFutureTransactionDate, constant.EntityTransaction)
	}

	if transactionStatus == constant.PENDING {
		return time.Time{}, pkg.ValidateBusinessError(constant.ErrInvalidPendingFutureTransactionDate, constant.EntityTransaction)
	}

	return transactionInput.TransactionDate.Time(), nil
}
