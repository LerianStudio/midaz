// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestValidatePerActionRouteTypes uses table-driven tests to validate per-action
// operation route type checking. Each distinct action must have at least one source
// and one destination route. Bidirectional routes count as both.
func TestValidatePerActionRouteTypes(t *testing.T) {
	t.Parallel()

	sourceRouteID := uuid.New()
	destRouteID := uuid.New()
	bidiRouteID := uuid.New()
	extraSourceID := uuid.New()
	extraDestID := uuid.New()

	operationRoutes := []*mmodel.OperationRoute{
		{ID: sourceRouteID, OperationType: "source"},
		{ID: destRouteID, OperationType: "destination"},
		{ID: bidiRouteID, OperationType: "bidirectional"},
		{ID: extraSourceID, OperationType: "source"},
		{ID: extraDestID, OperationType: "destination"},
	}

	routeByID := make(map[uuid.UUID]*mmodel.OperationRoute, len(operationRoutes))
	for _, r := range operationRoutes {
		routeByID[r.ID] = r
	}

	entityType := reflect.TypeOf(mmodel.TransactionRoute{}).Name()

	tests := []struct {
		name          string
		actionInputs  []mmodel.OperationRouteActionInput
		opRoutes      []*mmodel.OperationRoute
		expectedError error
	}{
		{
			name: "valid_multi_action_hold_and_commit_with_source_and_destination_each",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "hold", OperationRouteID: sourceRouteID},
				{Action: "hold", OperationRouteID: destRouteID},
				{Action: "commit", OperationRouteID: extraSourceID},
				{Action: "commit", OperationRouteID: extraDestID},
			},
			opRoutes:      operationRoutes,
			expectedError: nil,
		},
		{
			name: "valid_single_action_direct_with_source_and_destination",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "direct", OperationRouteID: sourceRouteID},
				{Action: "direct", OperationRouteID: destRouteID},
			},
			opRoutes:      operationRoutes,
			expectedError: nil,
		},
		{
			name: "valid_single_action_with_bidirectional_route_covers_both",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "direct", OperationRouteID: bidiRouteID},
			},
			opRoutes:      operationRoutes,
			expectedError: nil,
		},
		{
			name: "valid_incomplete_action_set_only_hold_and_commit",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "hold", OperationRouteID: sourceRouteID},
				{Action: "hold", OperationRouteID: destRouteID},
				{Action: "commit", OperationRouteID: extraSourceID},
				{Action: "commit", OperationRouteID: extraDestID},
			},
			opRoutes:      operationRoutes,
			expectedError: nil,
		},
		{
			name: "error_missing_source_for_hold_action",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "hold", OperationRouteID: destRouteID},
				{Action: "direct", OperationRouteID: sourceRouteID},
				{Action: "direct", OperationRouteID: destRouteID},
			},
			opRoutes:      operationRoutes,
			expectedError: pkg.ValidateBusinessError(constant.ErrNoSourceForAction, entityType, "hold"),
		},
		{
			name: "error_missing_destination_for_commit_action",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "commit", OperationRouteID: sourceRouteID},
				{Action: "direct", OperationRouteID: sourceRouteID},
				{Action: "direct", OperationRouteID: destRouteID},
			},
			opRoutes:      operationRoutes,
			expectedError: pkg.ValidateBusinessError(constant.ErrNoDestinationForAction, entityType, "commit"),
		},
		{
			name: "error_invalid_action_value",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "invalid_action", OperationRouteID: sourceRouteID},
				{Action: "direct", OperationRouteID: destRouteID},
			},
			opRoutes:      operationRoutes,
			expectedError: pkg.ValidateBusinessError(constant.ErrInvalidRouteAction, entityType, "invalid_action"),
		},
		{
			name: "error_duplicate_action_route_pair",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "direct", OperationRouteID: sourceRouteID},
				{Action: "direct", OperationRouteID: sourceRouteID},
				{Action: "direct", OperationRouteID: destRouteID},
			},
			opRoutes:      operationRoutes,
			expectedError: pkg.ValidateBusinessError(constant.ErrDuplicateActionRoute, entityType, sourceRouteID.String(), "direct"),
		},
		{
			name: "valid_same_route_different_actions_is_not_duplicate",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "direct", OperationRouteID: bidiRouteID},
				{Action: "hold", OperationRouteID: bidiRouteID},
			},
			opRoutes:      operationRoutes,
			expectedError: nil,
		},
		{
			name: "error_missing_source_and_destination_for_cancel_action",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "cancel", OperationRouteID: destRouteID},
				{Action: "direct", OperationRouteID: sourceRouteID},
				{Action: "direct", OperationRouteID: destRouteID},
			},
			opRoutes:      operationRoutes,
			expectedError: pkg.ValidateBusinessError(constant.ErrNoSourceForAction, entityType, "cancel"),
		},
		{
			name: "valid_bidirectional_satisfies_source_and_dest_for_multiple_actions",
			actionInputs: []mmodel.OperationRouteActionInput{
				{Action: "hold", OperationRouteID: bidiRouteID},
				{Action: "commit", OperationRouteID: bidiRouteID},
			},
			opRoutes:      operationRoutes,
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateOperationRouteTypes(tt.actionInputs, tt.opRoutes)

			if tt.expectedError == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			}
		})
	}
}
