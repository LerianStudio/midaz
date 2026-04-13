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

// TestValidateOperationRouteTypes_SourceDestinationCoverage uses table-driven tests
// to validate that operation routes have at least one source and one destination.
// Bidirectional routes count as both source and destination.
func TestValidateOperationRouteTypes_SourceDestinationCoverage(t *testing.T) {
	t.Parallel()

	sourceRouteID := uuid.New()
	destRouteID := uuid.New()
	bidiRouteID := uuid.New()
	extraSourceID := uuid.New()
	extraDestID := uuid.New()

	entityType := reflect.TypeOf(mmodel.TransactionRoute{}).Name()

	tests := []struct {
		name          string
		opRoutes      []*mmodel.OperationRoute
		expectedError error
	}{
		{
			name: "valid_source_and_destination",
			opRoutes: []*mmodel.OperationRoute{
				{ID: sourceRouteID, OperationType: "source"},
				{ID: destRouteID, OperationType: "destination"},
			},
			expectedError: nil,
		},
		{
			name: "valid_multiple_sources_and_destinations",
			opRoutes: []*mmodel.OperationRoute{
				{ID: sourceRouteID, OperationType: "source"},
				{ID: extraSourceID, OperationType: "source"},
				{ID: destRouteID, OperationType: "destination"},
				{ID: extraDestID, OperationType: "destination"},
			},
			expectedError: nil,
		},
		{
			name: "valid_bidirectional_covers_both",
			opRoutes: []*mmodel.OperationRoute{
				{ID: bidiRouteID, OperationType: "bidirectional"},
			},
			expectedError: nil,
		},
		{
			name: "valid_bidirectional_with_source",
			opRoutes: []*mmodel.OperationRoute{
				{ID: sourceRouteID, OperationType: "source"},
				{ID: bidiRouteID, OperationType: "bidirectional"},
			},
			expectedError: nil,
		},
		{
			name: "valid_bidirectional_with_destination",
			opRoutes: []*mmodel.OperationRoute{
				{ID: destRouteID, OperationType: "destination"},
				{ID: bidiRouteID, OperationType: "bidirectional"},
			},
			expectedError: nil,
		},
		{
			name:          "empty_routes_returns_error",
			opRoutes:      []*mmodel.OperationRoute{},
			expectedError: pkg.ValidateBusinessError(constant.ErrNoSourceForAction, entityType, ""),
		},
		{
			name:          "nil_routes_returns_error",
			opRoutes:      nil,
			expectedError: pkg.ValidateBusinessError(constant.ErrNoSourceForAction, entityType, ""),
		},
		{
			name: "error_only_sources_no_destination",
			opRoutes: []*mmodel.OperationRoute{
				{ID: sourceRouteID, OperationType: "source"},
				{ID: extraSourceID, OperationType: "source"},
			},
			expectedError: pkg.ValidateBusinessError(constant.ErrNoDestinationForAction, entityType, ""),
		},
		{
			name: "error_only_destinations_no_source",
			opRoutes: []*mmodel.OperationRoute{
				{ID: destRouteID, OperationType: "destination"},
				{ID: extraDestID, OperationType: "destination"},
			},
			expectedError: pkg.ValidateBusinessError(constant.ErrNoSourceForAction, entityType, ""),
		},
		{
			name: "error_single_source_no_destination",
			opRoutes: []*mmodel.OperationRoute{
				{ID: sourceRouteID, OperationType: "source"},
			},
			expectedError: pkg.ValidateBusinessError(constant.ErrNoDestinationForAction, entityType, ""),
		},
		{
			name: "error_single_destination_no_source",
			opRoutes: []*mmodel.OperationRoute{
				{ID: destRouteID, OperationType: "destination"},
			},
			expectedError: pkg.ValidateBusinessError(constant.ErrNoSourceForAction, entityType, ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateOperationRouteTypes(tt.opRoutes)

			if tt.expectedError == nil {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			}
		})
	}
}
