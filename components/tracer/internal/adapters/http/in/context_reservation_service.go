// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=context_reservation_service.go -destination=mocks/context_reservation_service_mock.go -package=mocks

type ContextReserveAdmitter interface {
	Execute(context.Context, tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error)
}

type ContextReserveCompleter interface {
	ExecuteReport(context.Context, uuid.UUID, model.ReserveOperationStatus) (*tracercontract.TransactionCompletionResult, error)
}

// ContextReserveIDCompleter treats a reservation ID as an address for its whole operation.
type ContextReserveIDCompleter interface {
	Execute(context.Context, uuid.UUID, model.ReserveOperationStatus) (*tracercontract.ReservationCompletionResult, error)
}
