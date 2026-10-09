// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// fixedTestTime is the deterministic timestamp the repository mocks stamp onto
// created entities. A wall-clock read would make any assertion over CreatedAt /
// UpdatedAt depend on when the suite runs.
var fixedTestTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// assertInvalidQueryParameterResponse asserts the 400 envelope a /v1 handler
// returns for a rejected query parameter. /v1 serves the v3 shape, so the human
// text is under "message"; a /v2 caller would read "detail" instead.
func assertInvalidQueryParameterResponse(t *testing.T, body []byte) {
	t.Helper()

	var errResp map[string]any
	err := json.Unmarshal(body, &errResp)
	require.NoError(t, err)

	assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), errResp["code"])
	assert.Equal(t, "Invalid Query Parameter", errResp["title"])
	assert.Contains(t, errResp["message"], "query parameters")
}

// assertEmptyOffsetPage asserts the 200 body an offset-paginated list returns
// when nothing matches: items is a present, non-null empty array and the
// requested limit and page are echoed.
func assertEmptyOffsetPage(t *testing.T, body []byte, wantLimit, wantPage int) {
	t.Helper()

	got := assertEmptyItems(t, body)

	assert.EqualValues(t, wantLimit, got["limit"], "body: %s", string(body))
	assert.EqualValues(t, wantPage, got["page"], "body: %s", string(body))
}

// assertEmptyCursorPage asserts the 200 body a cursor-paginated list returns
// when nothing matches: items is a present, non-null empty array, the requested
// limit is echoed and no cursor points past the empty page.
func assertEmptyCursorPage(t *testing.T, body []byte, wantLimit int) {
	t.Helper()

	got := assertEmptyItems(t, body)

	assert.EqualValues(t, wantLimit, got["limit"], "body: %s", string(body))
	assert.Empty(t, got["next_cursor"], "body: %s", string(body))
	assert.Empty(t, got["prev_cursor"], "body: %s", string(body))
}

func assertEmptyItems(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got), "body: %s", string(body))

	raw, present := got["items"]
	require.True(t, present, "items must be present, body: %s", string(body))

	items, ok := raw.([]any)
	require.True(t, ok, "items must be a JSON array, not null, body: %s", string(body))
	assert.Equal(t, []any{}, items)

	return got
}
