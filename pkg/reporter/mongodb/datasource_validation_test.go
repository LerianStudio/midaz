// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMongoFieldNames_AllowsKnownProjectionAndFilterFields(t *testing.T) {
	t.Parallel()

	known := map[string]bool{"name": true, "metadata": true}
	fields, err := validateMongoFieldNames("reports", known, []string{"name", "metadata.status"}, []string{"name"})
	require.NoError(t, err)
	assert.Equal(t, []string{"name", "metadata.status"}, fields)
}

func TestValidateMongoFieldNames_RejectsUnknownFields(t *testing.T) {
	t.Parallel()

	known := map[string]bool{"name": true}
	_, err := validateMongoFieldNames("reports", known, []string{"unknown"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid fields for collection \"reports\"")
}

func TestValidateMongoFieldNames_RejectsUnknownFilterFields(t *testing.T) {
	t.Parallel()

	known := map[string]bool{"name": true}
	_, err := validateMongoFieldNames("reports", known, []string{"*"}, []string{"unknown"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter fields for collection \"reports\"")
}
