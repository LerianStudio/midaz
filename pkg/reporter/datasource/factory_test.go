// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"testing"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewProvider builds the in-process single-tenant DirectProvider. The remote
// Fetcher branch and its config validation have been retired, so the factory
// now has a single outcome.
func TestNewProvider_ReturnsDirectProvider(t *testing.T) {
	pkg.ResetRegisteredDataSourceIDsForTesting()

	provider, err := NewProvider(ProviderConfig{
		SafeDataSources: pkg.NewSafeDataSources(nil),
	})

	require.NoError(t, err)
	require.NotNil(t, provider)

	_, ok := provider.(*DirectProvider)
	assert.True(t, ok, "expected *DirectProvider, got %T", provider)
}
