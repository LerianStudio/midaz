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

	if transactionInput.TransactionDate == nil || transactionInput.TransactionDate.IsZero() {
		return now, nil
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	if transactionInput.TransactionDate.After(now) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidFutureTransactionDate, constant.EntityTransaction)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction date validation failed", err)
		logger.Log(ctx, libLog.LevelWarn, "Transaction date cannot be a future date", libLog.Err(err))

		return time.Time{}, err
	}

	if transactionStatus == constant.PENDING {
		err := pkg.ValidateBusinessError(constant.ErrInvalidPendingFutureTransactionDate, constant.EntityTransaction)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction date validation failed", err)
		logger.Log(ctx, libLog.LevelWarn, "Pending transaction cannot have a custom transaction date", libLog.Err(err))

		return time.Time{}, err
	}

	return transactionInput.TransactionDate.Time(), nil
}
