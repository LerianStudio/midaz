// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"testing"

	midazhttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafePayloadSummary_RedactsValues(t *testing.T) {
	t.Parallel()

	payload := struct {
		Alias   string
		Key     string
		Account string
		Send    struct{ ID string }
	}{
		Alias:   "sensitive-alias",
		Key:     "super-secret-key",
		Account: "123456",
		Send:    struct{ ID string }{ID: "send-1"},
	}

	summary := safePayloadSummary(payload)

	assert.Contains(t, summary, "type=")
	assert.Contains(t, summary, "hasAlias=true")
	assert.Contains(t, summary, "hasKey=true")
	assert.Contains(t, summary, "hasSend=true")
	assert.NotContains(t, summary, "sensitive-alias")
	assert.NotContains(t, summary, "super-secret-key")
	assert.NotContains(t, summary, "123456")
}

func TestSafePayloadAttributes_RedactsValues(t *testing.T) {
	t.Parallel()

	payload := struct {
		Alias          string
		Key            string
		OrganizationID string
	}{
		Alias:          "sensitive-alias",
		Key:            "super-secret-key",
		OrganizationID: "org-123",
	}

	attrs := safePayloadAttributes(payload)
	require.NotEmpty(t, attrs)

	for _, attr := range attrs {
		serialized := fmt.Sprint(attr.Value.AsInterface())
		assert.NotContains(t, serialized, "sensitive-alias")
		assert.NotContains(t, serialized, "super-secret-key")
		assert.NotContains(t, serialized, "org-123")
	}
}

func TestSafeQueryAttributes_RedactsValues(t *testing.T) {
	t.Parallel()

	document := "12345678900"
	accountID := "acc-123"
	query := &midazhttp.QueryHeader{
		Limit:     25,
		SortOrder: "desc",
		Cursor:    "opaque-cursor",
		Document:  &document,
		AccountID: &accountID,
	}

	attrs := safeQueryAttributes(query)
	require.NotEmpty(t, attrs)

	for _, attr := range attrs {
		serialized := fmt.Sprint(attr.Value.AsInterface())
		assert.NotContains(t, serialized, "12345678900")
		assert.NotContains(t, serialized, "acc-123")
	}
}
