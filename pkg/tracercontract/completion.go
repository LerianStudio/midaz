// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracercontract

import "github.com/google/uuid"

// CompletionRequest acknowledges the coordinated reservation contract. The
// transaction/reservation identity remains in the existing transport path.
type CompletionRequest struct {
	ContractRevision string `json:"contractRevision"`
}

// TransactionCompletionResult reports a known producer outcome. EvaluationID
// is absent when completion precedes admission; that is not an ALLOW decision.
// Flipped counts actual capacity transitions in this call (zero on replay).
type TransactionCompletionResult struct {
	ContractRevision string     `json:"contractRevision"`
	TransactionID    uuid.UUID  `json:"transactionId"`
	Status           string     `json:"status"`
	Flipped          int        `json:"flipped"`
	EvaluationID     *uuid.UUID `json:"evaluationId,omitempty"`
}

func (r TransactionCompletionResult) Validate() error {
	if r.ContractRevision != ReserveContractRevision || r.TransactionID == uuid.Nil || r.Flipped < 0 {
		return invalid("completion result identity")
	}

	if r.Status != "CONFIRMED" && r.Status != "RELEASED" {
		return invalid("completion status")
	}

	if r.EvaluationID != nil && *r.EvaluationID == uuid.Nil {
		return invalid("completion evaluation")
	}

	if r.EvaluationID == nil && r.Flipped != 0 {
		return invalid("completion without evaluation")
	}

	return nil
}
