// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
)

type balanceEngineResponse struct {
	result *engine.Result
	err    error
}

type scriptedBalanceEngine struct {
	requests  []EngineExecution
	contexts  []context.Context
	responses []balanceEngineResponse
}

var _ BalanceEngine = (*scriptedBalanceEngine)(nil)

func (s *scriptedBalanceEngine) Execute(ctx context.Context, input EngineExecution) (*engine.Result, error) {
	s.requests = append(s.requests, input)
	s.contexts = append(s.contexts, ctx)

	if len(s.responses) == 0 {
		return nil, errors.New("unexpected balance engine execution")
	}

	response := s.responses[0]
	s.responses = s.responses[1:]

	return response.result, response.err
}

func TestBalanceEnginePortPreservesExecutionAndOutcomes(t *testing.T) {
	t.Parallel()

	transactionID := uuid.MustParse("ff06f52d-d6e3-425f-83ab-58b7ae73145a")
	execution := EngineExecution{
		Request: engine.Request{
			OrganizationID: uuid.MustParse("81d280ef-824f-48be-b804-a7a9472d3303"),
			LedgerID:       uuid.MustParse("090217e0-7be5-4a0e-b3f7-44d700cf7b86"),
			ExecutionID:    uuid.MustParse("656e9cf1-17c9-4257-83f6-687d0f18dd31"),
		},
		IntentFingerprint: "immutable-intent",
		Guards: []ExecutionGuard{{
			TransactionID: transactionID,
			ExpectedToken: "pending",
			NextToken:     "committed",
		}},
		Recovery: []RecoveryIntent{{
			TransactionID: transactionID,
			Payload:       json.RawMessage(`{"status":"COMMITTED"}`),
		}},
	}
	result := &engine.Result{}
	failure := &engine.Failure{
		Code:             engine.FailureStaleVersion,
		TransactionIndex: 0,
		PostingIndex:     1,
		BalanceRef:       "source",
	}
	technicalCause := errors.New("connection lost")
	technicalErr := fmt.Errorf("execute balances: %w", technicalCause)
	stub := &scriptedBalanceEngine{responses: []balanceEngineResponse{
		{result: result},
		{err: failure},
		{err: technicalErr},
	}}
	useCase := UseCase{BalanceEngine: stub}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got, err := useCase.BalanceEngine.Execute(ctx, execution)
	if err != nil || got != result {
		t.Fatalf("successful execution = (%p, %v), want (%p, nil)", got, err, result)
	}

	got, err = useCase.BalanceEngine.Execute(ctx, execution)
	var gotFailure *engine.Failure
	if got != nil || !errors.As(err, &gotFailure) || gotFailure != failure || !errors.Is(err, failure) {
		t.Fatalf("business failure = (%v, %v), want original typed failure", got, err)
	}

	got, err = useCase.BalanceEngine.Execute(ctx, execution)
	if got != nil || err != technicalErr || !errors.Is(err, technicalCause) {
		t.Fatalf("technical failure = (%v, %v), want original wrapped cause", got, err)
	}
	gotFailure = nil
	if errors.As(err, &gotFailure) {
		t.Fatal("technical failure must not become an engine failure")
	}

	if len(stub.requests) != 3 || len(stub.contexts) != 3 || len(stub.responses) != 0 {
		t.Fatalf("unexpected invocation counts: requests=%d contexts=%d remaining=%d", len(stub.requests), len(stub.contexts), len(stub.responses))
	}
	for i := range stub.requests {
		if !reflect.DeepEqual(stub.requests[i], execution) || stub.contexts[i] != ctx {
			t.Errorf("invocation %d changed execution or context", i)
		}
	}
}

func TestBalanceEnginePortDefaultsToDisabled(t *testing.T) {
	t.Parallel()

	var useCase UseCase
	if useCase.BalanceEngine != nil {
		t.Fatal("zero-value use case must not enable balance engine execution")
	}
}
