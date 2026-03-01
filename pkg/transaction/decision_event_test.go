// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeDecisionLifecycleEvent(t *testing.T) {
	event := DecisionLifecycleEvent{}

	normalized := NormalizeDecisionLifecycleEvent(event)

	assert.Equal(t, DecisionEndpointModeSync, normalized.DecisionContract.EndpointMode)
	assert.Equal(t, GuaranteeModelSourceDurable, normalized.DecisionContract.GuaranteeModel)
	assert.Equal(t, DestinationBlockedPolicyCreditAnyway, normalized.DecisionContract.DestinationBlockedPolicy)
	assert.Equal(t, int64(150), normalized.DecisionContract.DecisionLatencySLOMs)
}
