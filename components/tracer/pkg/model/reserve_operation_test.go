// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestReserveOperationIdentityDoesNotDependOnRequestID(t *testing.T) {
	t.Parallel()
	key := model.ReserveOperationKey{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(74001), RequestID: testutil.MustDeterministicUUID(74002)}
	identity := key.Identity()
	require.NoError(t, identity.Validate())
	key.RequestID = testutil.MustDeterministicUUID(74003)
	require.Equal(t, identity, key.Identity())
	for _, bad := range []model.ReserveOperationIdentity{{}, {IntegrationID: "producer"}, {IntegrationID: " producer", TransactionID: key.TransactionID}, {IntegrationID: "producer\x00a", TransactionID: key.TransactionID}, {IntegrationID: "producer\xff", TransactionID: key.TransactionID}} {
		require.ErrorIs(t, bad.Validate(), constant.ErrInvalidRequestBody)
	}
	key.RequestID = uuid.Nil
	require.NoError(t, key.Identity().Validate(), "completion has no request identifier")
	require.ErrorIs(t, key.Validate(), constant.ErrInvalidRequestBody)
}

func TestReserveOperationStateNeverExpiresUnknownOutcome(t *testing.T) {
	t.Parallel()
	at := testutil.FixedTime()
	zero := time.Time{}
	for _, tt := range []struct {
		name  string
		state model.ReserveOperationState
		valid bool
	}{
		{"open", model.ReserveOperationState{Status: model.OperationOpen}, true},
		{"confirmed", model.ReserveOperationState{Status: model.OperationConfirmed, CompletedAt: &at}, true},
		{"released", model.ReserveOperationState{Status: model.OperationReleased, CompletedAt: &at}, true},
		{"unknown", model.ReserveOperationState{}, false},
		{"no ttl completion", model.ReserveOperationState{Status: "EXPIRED", CompletedAt: &at}, false},
		{"missing completion time", model.ReserveOperationState{Status: model.OperationConfirmed}, false},
		{"zero completion time", model.ReserveOperationState{Status: model.OperationReleased, CompletedAt: &zero}, false},
		{"open cannot be completed", model.ReserveOperationState{Status: model.OperationOpen, CompletedAt: &at}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
			}
		})
	}
}
