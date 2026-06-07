// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/pkg"
	"go.opentelemetry.io/otel/trace"
)

type UseCase struct {
	HolderRepo     holder.Repository
	InstrumentRepo instrument.Repository
}

// recordSpanError records err onto the span using the class-appropriate helper:
// business/4xx errors keep the span status UNSET via HandleSpanBusinessErrorEvent,
// technical/5xx errors flip it red via HandleSpanError (telemetry rule T5).
func recordSpanError(span trace.Span, message string, err error) {
	if pkg.IsBusinessError(err) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, message, err)
		return
	}

	libOpentelemetry.HandleSpanError(span, message, err)
}
