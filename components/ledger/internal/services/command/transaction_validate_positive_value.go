// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// validatePositiveTransactionValue rejects zero and negative transaction totals
// before the idempotency slot or any balance state is touched.
func validatePositiveTransactionValue(ctx context.Context, span trace.Span, logger libLog.Logger, value decimal.Decimal) error {
	if value.IsPositive() {
		return nil
	}

	err := pkg.ValidateBusinessError(constant.ErrInvalidTransactionNonPositiveValue, constant.EntityTransaction)
	libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction value must be greater than zero", err)
	logger.Log(ctx, libLog.LevelWarn, "Transaction value must be greater than zero", libLog.Err(err))

	return err
}
