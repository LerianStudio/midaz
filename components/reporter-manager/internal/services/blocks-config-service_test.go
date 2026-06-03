// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestUseCase_GetBlocksConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "Success - returns blocks config with 13 block types",
			test: func(t *testing.T) {
				t.Helper()

				uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
				result := uc.GetBlocksConfig(context.Background())

				require.NotNil(t, result, "result must not be nil")
				assert.Len(t, result.Blocks, 13, "must return exactly 13 block types")
			},
		},
		{
			name: "Success - counter block has correct properties",
			test: func(t *testing.T) {
				t.Helper()

				uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
				result := uc.GetBlocksConfig(context.Background())
				require.NotNil(t, result)

				var found bool

				for _, block := range result.Blocks {
					if block.Type == "counter" {
						found = true
						assert.Equal(t, "dimp", block.Category)
						assert.False(t, block.AcceptsChildren)
						require.Len(t, block.Properties, 2)

						break
					}
				}

				assert.True(t, found, "counter block must exist")
			},
		},
		{
			name: "Success - section block accepts children",
			test: func(t *testing.T) {
				t.Helper()

				uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
				result := uc.GetBlocksConfig(context.Background())
				require.NotNil(t, result)

				var found bool

				for _, block := range result.Blocks {
					if block.Type == "section" {
						found = true
						assert.True(t, block.AcceptsChildren, "section block must accept children")

						break
					}
				}

				assert.True(t, found, "section block must exist")
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.test(t)
		})
	}
}

func TestUseCase_GetFiltersConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "Success - returns filters config with DIMP filters",
			test: func(t *testing.T) {
				t.Helper()

				uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
				result := uc.GetFiltersConfig(context.Background())

				require.NotNil(t, result, "result must not be nil")
				require.NotEmpty(t, result.Filters, "must return at least one filter")

				filterNames := make([]string, len(result.Filters))
				for i, f := range result.Filters {
					filterNames[i] = f.Name
				}

				dimpFilters := []string{"replace", "where", "sum", "count"}
				for _, df := range dimpFilters {
					assert.Contains(t, filterNames, df, "must contain DIMP filter: %s", df)
				}
			},
		},
		{
			name: "Success - every filter has required fields",
			test: func(t *testing.T) {
				t.Helper()

				uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
				result := uc.GetFiltersConfig(context.Background())
				require.NotNil(t, result)

				for _, filter := range result.Filters {
					assert.NotEmpty(t, filter.Name)
					assert.NotEmpty(t, filter.Description)
					assert.NotNil(t, filter.Args)
					assert.NotEmpty(t, filter.Example)
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.test(t)
		})
	}
}
