// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/google/uuid"

	traceradapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=tracer_context_ports.go -destination=mock_tracer_context_ports_test.go -package=command

// ContextTracerClient exposes only the complete shared contract. Completion
// is addressed by transaction, so it also settles an admission whose response
// was lost. The client cannot execute accounting.
type ContextTracerClient interface {
	Reserve(context.Context, tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error)
	ConfirmByTransaction(context.Context, uuid.UUID) (*tracercontract.TransactionCompletionResult, error)
	ReleaseByTransaction(context.Context, uuid.UUID) (*tracercontract.TransactionCompletionResult, error)
}

// TracerFactsLoader reads official records only after participation/skip gates.
type TracerFactsLoader interface {
	EvaluationContext(context.Context, uuid.UUID, uuid.UUID, []traceradapter.PreparedEntry) (tracercontract.Context, error)
}
