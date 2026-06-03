// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/template_builder"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestUseCase_GenerateCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     *template_builder.GenerateCodeInput
		expectErr bool
		validate  func(t *testing.T, resp *template_builder.GenerateCodeResponse)
	}{
		{
			name: "Success - generates code for text block",
			input: &template_builder.GenerateCodeInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: "Hello World"},
				},
				Format: "",
			},
			expectErr: false,
			validate: func(t *testing.T, resp *template_builder.GenerateCodeResponse) {
				t.Helper()
				assert.Equal(t, "Hello World\n", resp.Code)
				assert.NotNil(t, resp.MappedFields)
			},
		},
		{
			name: "Success - generates code with variable and mapped fields",
			input: &template_builder.GenerateCodeInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "variable", Variable: "orders.total"},
				},
				Format: "",
			},
			expectErr: false,
			validate: func(t *testing.T, resp *template_builder.GenerateCodeResponse) {
				t.Helper()
				assert.Contains(t, resp.Code, "{{ orders.total }}")
				require.Contains(t, resp.MappedFields, "default")
				require.Contains(t, resp.MappedFields["default"], "orders")
				assert.Contains(t, resp.MappedFields["default"]["orders"], "total")
			},
		},
		{
			name: "Success - generates code with HTML format",
			input: &template_builder.GenerateCodeInput{
				Blocks: []template_builder.TemplateBlock{
					{Type: "text", Content: "Report"},
				},
				Format: "html",
			},
			expectErr: false,
			validate: func(t *testing.T, resp *template_builder.GenerateCodeResponse) {
				t.Helper()
				assert.Contains(t, resp.Code, "<!DOCTYPE html>")
				assert.Contains(t, resp.Code, "Report")
			},
		},
		{
			name:      "Error - nil input",
			input:     nil,
			expectErr: true,
			validate:  nil,
		},
		{
			name: "Error - empty blocks",
			input: &template_builder.GenerateCodeInput{
				Blocks: []template_builder.TemplateBlock{},
				Format: "",
			},
			expectErr: true,
			validate:  nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			uc := &UseCase{Logger: log.NewNop(), Tracer: noop.NewTracerProvider().Tracer("test")}
			resp, err := uc.GenerateCode(context.Background(), tt.input)

			if tt.expectErr {
				require.Error(t, err)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				tt.validate(t, resp)
			}
		})
	}
}
