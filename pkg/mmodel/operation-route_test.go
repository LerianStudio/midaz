// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestOperationRoute_ActionField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		operationRoute OperationRoute
		expectedAction string
	}{
		{
			name: "direct action",
			operationRoute: OperationRoute{
				ID:            uuid.New(),
				OperationType: "source",
				Action:        "direct",
			},
			expectedAction: "direct",
		},
		{
			name: "hold action",
			operationRoute: OperationRoute{
				ID:            uuid.New(),
				OperationType: "destination",
				Action:        "hold",
			},
			expectedAction: "hold",
		},
		{
			name: "empty action",
			operationRoute: OperationRoute{
				ID:            uuid.New(),
				OperationType: "source",
				Action:        "",
			},
			expectedAction: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expectedAction, tt.operationRoute.Action)
		})
	}
}
