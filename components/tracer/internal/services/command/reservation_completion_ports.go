// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
	"github.com/google/uuid"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=reservation_completion_ports.go -destination=mocks/reservation_completion_ports_mock.go -package=mocks

// ReservationOperationLocator reads immutable ownership from the tenant primary.
type ReservationOperationLocator interface {
	GetReservationOwner(context.Context, string, uuid.UUID) (*model.ReserveReservationOwner, error)
}

type ReserveOperationReporter interface {
	ExecuteReport(context.Context, uuid.UUID, model.ReserveOperationStatus) (*tracercontract.TransactionCompletionResult, error)
}
