// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// assertInvalidQueryParameterResponse asserts the canonical 400 envelope every
// handler returns for a rejected query parameter. Shared by the holder, instrument
// and transaction table tests.
func assertInvalidQueryParameterResponse(t *testing.T, body []byte) {
	t.Helper()

	var errResp map[string]any
	err := json.Unmarshal(body, &errResp)
	require.NoError(t, err)

	assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), errResp["code"])
	assert.Equal(t, "Invalid Query Parameter", errResp["title"])
	assert.Contains(t, errResp["detail"], "query parameters")
}
