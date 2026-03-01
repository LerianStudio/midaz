// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

// DecisionLifecycleAction identifies decision/posting lifecycle checkpoints.
type DecisionLifecycleAction string

// DecisionLifecycleAction constants enumerate the checkpoints in a decision/posting lifecycle.
const (
	DecisionLifecycleActionAuthorizationRequested DecisionLifecycleAction = "authorization_requested"
	DecisionLifecycleActionAuthorizationApproved  DecisionLifecycleAction = "authorization_approved"
	DecisionLifecycleActionAuthorizationDeclined  DecisionLifecycleAction = "authorization_declined"
	DecisionLifecycleActionPostingCompleted       DecisionLifecycleAction = "posting_completed"
	DecisionLifecycleActionPostingFailed          DecisionLifecycleAction = "posting_failed"
)

// DecisionLifecycleEvent captures decision contract semantics together with a lifecycle checkpoint.
type DecisionLifecycleEvent struct {
	TransactionID     string                  `json:"transactionId"`
	Action            DecisionLifecycleAction `json:"action"`
	DecisionContract  DecisionContract        `json:"decisionContract"`
	TransactionStatus string                  `json:"transactionStatus"`
}

// NormalizeDecisionLifecycleEvent ensures event payload defaults are stable.
func NormalizeDecisionLifecycleEvent(event DecisionLifecycleEvent) DecisionLifecycleEvent {
	event.DecisionContract = event.DecisionContract.Normalize()

	return event
}
