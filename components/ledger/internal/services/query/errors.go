// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
)

// recordQueryError records err on span with the helper that matches its class (T5) and
// logs it only when it is an infrastructure error (T7, T8). A business/4xx error keeps the
// span green with a business event and is not logged: the HTTP access log already emits the
// single line for it. A technical/5xx error flips the span red and is logged once at Error,
// here, because the use case is the innermost layer that owns it. fields are appended to the
// log call before the mandatory libLog.Err(err).
func recordQueryError(ctx context.Context, span trace.Span, logger libLog.Logger, message string, err error, fields ...libLog.Field) {
	if err == nil {
		return
	}

	spanattr.HandleSpanByErrorClass(span, message, err)

	if pkg.IsBusinessError(err) {
		return
	}

	fields = append(fields, libLog.Err(err))

	logger.Log(ctx, libLog.LevelError, message, fields)
}
