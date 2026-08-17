// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeeOperationRouteIDsArePublishedAsOptionalUUIDs(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	feeSchema := componentSchema(api, "Fee")
	require.NotNil(t, feeSchema)

	for _, field := range []string{"operationRouteFromId", "operationRouteToId"} {
		property, ok := feeSchema.Properties[field]
		require.Truef(t, ok, "Fee must publish %s", field)
		require.NotNil(t, property)
		assert.Equal(t, "uuid", property.Format)
		assert.NotContains(t, feeSchema.Required, field, "%s must stay optional", field)
	}
}
