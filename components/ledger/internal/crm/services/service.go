// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"github.com/LerianStudio/lib-observability/metrics"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/pkg"
	"go.opentelemetry.io/otel/trace"
)

type UseCase struct {
	HolderRepo     holder.Repository
	InstrumentRepo instrument.Repository

	// Idempotency backs the CRM idempotency claim/set methods over the shared
	// Redis infrastructure. A nil value disables the feature: claims become
	// no-op passthroughs (mirroring the streaming nil-emitter guard).
	Idempotency IdempotencyRepo

	// LedgerAccounts verifies an instrument's body-supplied ledger/account
	// references exist within the request organization before the instrument is
	// persisted. It is a hard dependency of CreateInstrument: a nil value is a
	// wiring error, not a disable switch, because the referential check is a
	// data-integrity guarantee rather than optional telemetry.
	LedgerAccounts LedgerAccountReader

	// MetricsFactory emits the bounded domain_operations_total /
	// domain_operation_duration_ms metrics for every state-mutating CRM
	// entrypoint via utils.RecordDomainOperation. A nil value is a no-op so the
	// binary runs with telemetry disabled.
	MetricsFactory *metrics.MetricsFactory
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
