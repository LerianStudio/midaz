// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import "strings"

const defaultDecisionLatencySLOMs int64 = 150

// DecisionEndpointMode defines how transaction authorization decisions are returned to callers.
type DecisionEndpointMode string

const (
	// DecisionEndpointModeSync means the API waits for an authorization decision.
	DecisionEndpointModeSync DecisionEndpointMode = "sync"
)

// GuaranteeModel describes what is guaranteed when a transaction is approved.
type GuaranteeModel string

const (
	// GuaranteeModelSourceDurable means approval requires durable source-side reservation.
	GuaranteeModelSourceDurable GuaranteeModel = "source_durable"
)

// DestinationBlockedPolicy defines what to do when destination policy changes after approval.
type DestinationBlockedPolicy string

const (
	// DestinationBlockedPolicyCreditAnyway keeps payment guarantee by still crediting funds.
	DestinationBlockedPolicyCreditAnyway DestinationBlockedPolicy = "credit_anyway"
)

// DecisionContract captures decision and guarantee semantics attached to processing payloads.
type DecisionContract struct {
	EndpointMode             DecisionEndpointMode     `json:"endpointMode" msgpack:"EndpointMode"`
	GuaranteeModel           GuaranteeModel           `json:"guaranteeModel" msgpack:"GuaranteeModel"`
	DestinationBlockedPolicy DestinationBlockedPolicy `json:"destinationBlockedPolicy" msgpack:"DestinationBlockedPolicy"`
	DecisionLatencySLOMs     int64                    `json:"decisionLatencySloMs" msgpack:"DecisionLatencySLOMs"`
}

// DefaultDecisionContract returns the platform default decision semantics.
func DefaultDecisionContract() DecisionContract {
	return DecisionContract{
		EndpointMode:             DecisionEndpointModeSync,
		GuaranteeModel:           GuaranteeModelSourceDurable,
		DestinationBlockedPolicy: DestinationBlockedPolicyCreditAnyway,
		DecisionLatencySLOMs:     defaultDecisionLatencySLOMs,
	}
}

// Normalize fills missing or invalid values with defaults.
func (c DecisionContract) Normalize() DecisionContract {
	defaults := DefaultDecisionContract()

	if strings.TrimSpace(string(c.EndpointMode)) == "" {
		c.EndpointMode = defaults.EndpointMode
	}

	if strings.TrimSpace(string(c.GuaranteeModel)) == "" {
		c.GuaranteeModel = defaults.GuaranteeModel
	}

	if strings.TrimSpace(string(c.DestinationBlockedPolicy)) == "" {
		c.DestinationBlockedPolicy = defaults.DestinationBlockedPolicy
	}

	if c.DecisionLatencySLOMs <= 0 {
		c.DecisionLatencySLOMs = defaults.DecisionLatencySLOMs
	}

	return c
}

// IsZero reports whether no meaningful contract fields were provided.
func (c DecisionContract) IsZero() bool {
	return strings.TrimSpace(string(c.EndpointMode)) == "" &&
		strings.TrimSpace(string(c.GuaranteeModel)) == "" &&
		strings.TrimSpace(string(c.DestinationBlockedPolicy)) == "" &&
		c.DecisionLatencySLOMs == 0
}
