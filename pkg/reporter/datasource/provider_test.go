// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDataSourceProviderInterfaceContract verifies that the DataSourceProvider
// interface defines the correct methods with expected signatures.
func TestDataSourceProviderInterfaceContract(t *testing.T) {
	t.Parallel()

	// Verify that a nil DataSourceProvider can be declared.
	// This proves the interface type exists and is usable.
	var provider DataSourceProvider
	assert.Nil(t, provider, "nil DataSourceProvider should be declarable")
}

func TestDataSourceProvider_ListDataSources_Signature(t *testing.T) {
	t.Parallel()

	// Verify method signature via method expression (avoids nil interface panic).
	// Compile-time check: (ctx) -> ([]DataSourceInfo, error)
	var _ func(DataSourceProvider, context.Context) ([]DataSourceInfo, error) = DataSourceProvider.ListDataSources
}

func TestDataSourceProvider_GetDataSourceSchema_Signature(t *testing.T) {
	t.Parallel()

	// Verify method signature via method expression (avoids nil interface panic).
	// Compile-time check: (ctx, dataSourceID) -> (*DataSourceSchema, error)
	var _ func(DataSourceProvider, context.Context, string) (*DataSourceSchema, error) = DataSourceProvider.GetDataSourceSchema
}

func TestDataSourceProvider_ValidateSchema_Signature(t *testing.T) {
	t.Parallel()

	// Verify method signature via method expression (avoids nil interface panic).
	// Compile-time check: (ctx, dataSourceID, tableFields) -> (*ValidationResult, error)
	var _ func(DataSourceProvider, context.Context, string, map[string][]string) (*ValidationResult, error) = DataSourceProvider.ValidateSchema
}

func TestDataSourceProvider_HealthCheck_Signature(t *testing.T) {
	t.Parallel()

	// Verify method signature via method expression (avoids nil interface panic).
	// Compile-time check: (ctx) -> (map[string]bool, error)
	var _ func(DataSourceProvider, context.Context) (map[string]bool, error) = DataSourceProvider.HealthCheck
}
