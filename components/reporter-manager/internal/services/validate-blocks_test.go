// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/template_builder"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestUseCase_ValidateBlocks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     *template_builder.ValidateBlocksInput
		wantValid bool
		errCount  int
	}{
		{
			name: "Success - valid text block",
			input: &template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: "Hello World"},
				},
			},
			wantValid: true,
			errCount:  0,
		},
		{
			name: "Success - valid variable block",
			input: &template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "variable", Variable: "name"},
				},
			},
			wantValid: true,
			errCount:  0,
		},
		{
			name: "Error - empty text content",
			input: &template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: ""},
				},
			},
			wantValid: false,
			errCount:  1,
		},
		{
			name: "Error - invalid block type",
			input: &template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "invalid"},
				},
			},
			wantValid: false,
			errCount:  1,
		},
		{
			name:      "Error - nil input",
			input:     nil,
			wantValid: false,
			errCount:  1,
		},
		{
			name: "Error - multiple invalid blocks",
			input: &template_builder.ValidateBlocksInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: ""},
					{Type: "variable", Variable: ""},
				},
			},
			wantValid: false,
			errCount:  2,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			resp := uc.ValidateBlocks(context.Background(), tt.input)

			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)
			assert.Len(t, resp.Errors, tt.errCount)
		})
	}
}
