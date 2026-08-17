// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package streaming

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var topicPattern = regexp.MustCompile(`^ledger\.[a-z0-9_]+\.[a-z0-9_]+$`)

func newTestCatalog(t *testing.T) libStreaming.Catalog {
	t.Helper()

	catalog, err := libStreaming.NewCatalog(
		libStreaming.EventDefinition{
			Key:           "account.created",
			ResourceType:  "account",
			EventType:     "created",
			SchemaVersion: "1.0.0",
		},
		libStreaming.EventDefinition{
			Key:           "operation_route.created",
			ResourceType:  "operation_route",
			EventType:     "created",
			SchemaVersion: "1.0.0",
		},
	)
	require.NoError(t, err)

	return catalog
}

func TestNewManifestHTTPHandler(t *testing.T) {
	t.Parallel()

	t.Run("happy path serves manifest document", func(t *testing.T) {
		t.Parallel()

		catalog := newTestCatalog(t)

		handler, err := NewManifestHTTPHandler("ledger", "ledger", catalog)
		require.NoError(t, err)
		require.NotNil(t, handler)

		req := httptest.NewRequest(http.MethodGet, ManifestRoutePath, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

		var doc struct {
			Publisher struct {
				ServiceName string `json:"serviceName"`
				SourceBase  string `json:"sourceBase"`
			} `json:"publisher"`
			Events []struct {
				Topic string `json:"topic"`
			} `json:"events"`
		}

		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
		assert.Equal(t, "ledger", doc.Publisher.ServiceName)
		assert.Equal(t, "ledger", doc.Publisher.SourceBase)
		require.NotEmpty(t, doc.Events)

		for _, e := range doc.Events {
			assert.Regexp(t, topicPattern, e.Topic, "expected every event topic to be underscore-canonical")
		}
	})

	t.Run("error path", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name        string
			serviceName string
			sourceBase  string
		}{
			{name: "empty service name", serviceName: "", sourceBase: "ledger"},
			{name: "empty source base", serviceName: "ledger", sourceBase: ""},
		}

		for _, tt := range tests {
			tt := tt

			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				handler, err := NewManifestHTTPHandler(tt.serviceName, tt.sourceBase, newTestCatalog(t))
				require.Error(t, err)
				assert.Nil(t, handler)
			})
		}
	})
}
