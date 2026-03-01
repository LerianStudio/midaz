// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultDecisionContract(t *testing.T) {
	contract := DefaultDecisionContract()

	assert.Equal(t, DecisionEndpointModeSync, contract.EndpointMode)
	assert.Equal(t, GuaranteeModelSourceDurable, contract.GuaranteeModel)
	assert.Equal(t, DestinationBlockedPolicyCreditAnyway, contract.DestinationBlockedPolicy)
	assert.Equal(t, int64(150), contract.DecisionLatencySLOMs)
}

func TestDecisionContractNormalize(t *testing.T) {
	contract := DecisionContract{}

	normalized := contract.Normalize()

	assert.Equal(t, DecisionEndpointModeSync, normalized.EndpointMode)
	assert.Equal(t, GuaranteeModelSourceDurable, normalized.GuaranteeModel)
	assert.Equal(t, DestinationBlockedPolicyCreditAnyway, normalized.DestinationBlockedPolicy)
	assert.Equal(t, int64(150), normalized.DecisionLatencySLOMs)
}

func TestDecisionContractIsZero(t *testing.T) {
	assert.True(t, DecisionContract{}.IsZero())
	assert.False(t, DecisionContract{DecisionLatencySLOMs: 150}.IsZero())
}
