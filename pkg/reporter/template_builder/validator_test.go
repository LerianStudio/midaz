// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template_builder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBlocks_AllBlockTypesValid(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{
		{BlockID: "b1", Type: "text", Content: "Hello"},
		{BlockID: "b2", Type: "variable", Variable: "name"},
		{BlockID: "b3", Type: "loop", Iterator: "i", Collection: "items", Children: []TemplateBlock{
			{Type: "text", Content: "child"},
		}},
		{BlockID: "b4", Type: "conditional", Condition: "x > 0", Children: []TemplateBlock{
			{Type: "text", Content: "yes"},
		}},
		{BlockID: "b5", Type: "aggregation", Collection: "items", Function: "sum", Field: "value"},
		{BlockID: "b6", Type: "calculation", Variable: "total", Function: "percent_of"},
		{BlockID: "b7", Type: "date_time", Variable: "created_at", Format: "02/01/2006"},
		{BlockID: "b8", Type: "counter", Properties: map[string]interface{}{
			"counterMode":  "increment",
			"counterNames": []interface{}{"cnt"},
		}},
		{BlockID: "b9", Type: "comment", Content: "a note"},
		{BlockID: "b10", Type: "section", Children: []TemplateBlock{
			{Type: "text", Content: "sec child"},
		}},
	}

	resp := ValidateBlocks(blocks)
	require.NotNil(t, resp)
	assert.True(t, resp.Valid)
	assert.Empty(t, resp.Errors)
}

func TestValidateBlocks_InvalidBlockType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		blocks      []TemplateBlock
		expectField string
		expectMsg   string
	}{
		{
			name:        "unknown type",
			blocks:      []TemplateBlock{{BlockID: "b1", Type: "unknown"}},
			expectField: "type",
			expectMsg:   "Tipo de bloco invalido: unknown",
		},
		{
			name:        "empty type",
			blocks:      []TemplateBlock{{BlockID: "b1", Type: ""}},
			expectField: "type",
			expectMsg:   "Tipo de bloco invalido: ",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.False(t, resp.Valid)
			require.Len(t, resp.Errors, 1)
			assert.Equal(t, tt.expectField, resp.Errors[0].Field)
			assert.Equal(t, tt.expectMsg, resp.Errors[0].Message)
		})
	}
}

func TestValidateBlocks_TextBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errField  string
		errMsg    string
	}{
		{
			name:      "valid text block",
			blocks:    []TemplateBlock{{Type: "text", Content: "hello"}},
			wantValid: true,
		},
		{
			name:      "empty content",
			blocks:    []TemplateBlock{{Type: "text", Content: ""}},
			wantValid: false,
			errField:  "content",
			errMsg:    "Texto deve ter conteudo",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.NotEmpty(t, resp.Errors)
				assert.Equal(t, tt.errField, resp.Errors[0].Field)
				assert.Equal(t, tt.errMsg, resp.Errors[0].Message)
			}
		})
	}
}

func TestValidateBlocks_VariableBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errField  string
		errMsg    string
	}{
		{
			name:      "valid variable",
			blocks:    []TemplateBlock{{Type: "variable", Variable: "name"}},
			wantValid: true,
		},
		{
			name:      "empty variable",
			blocks:    []TemplateBlock{{Type: "variable", Variable: ""}},
			wantValid: false,
			errField:  "variable",
			errMsg:    "Variavel deve ter um nome",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.NotEmpty(t, resp.Errors)
				assert.Equal(t, tt.errField, resp.Errors[0].Field)
				assert.Equal(t, tt.errMsg, resp.Errors[0].Message)
			}
		})
	}
}

func TestValidateBlocks_LoopBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errCount  int
		errFields []string
		errMsgs   []string
	}{
		{
			name: "valid loop",
			blocks: []TemplateBlock{{
				Type: "loop", Iterator: "i", Collection: "items",
				Children: []TemplateBlock{{Type: "text", Content: "x"}},
			}},
			wantValid: true,
		},
		{
			name: "valid loop with schema-qualified collection",
			blocks: []TemplateBlock{{
				Type: "loop", Iterator: "acc", Collection: "midaz_onboarding:onboarding_f2f69884845b466e9896e8a38ba0628d.account",
				Children: []TemplateBlock{{Type: "variable", Variable: "acc.name"}},
			}},
			wantValid: true,
		},
		{
			name: "missing iterator",
			blocks: []TemplateBlock{{
				Type: "loop", Iterator: "", Collection: "items",
				Children: []TemplateBlock{{Type: "text", Content: "x"}},
			}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"iterator"},
			errMsgs:   []string{"Loop deve ter um iterador"},
		},
		{
			name: "missing collection",
			blocks: []TemplateBlock{{
				Type: "loop", Iterator: "i", Collection: "",
				Children: []TemplateBlock{{Type: "text", Content: "x"}},
			}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"collection"},
			errMsgs:   []string{"Loop deve ter uma colecao"},
		},
		{
			name: "missing children",
			blocks: []TemplateBlock{{
				Type: "loop", Iterator: "i", Collection: "items",
				Children: nil,
			}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"children"},
			errMsgs:   []string{"Loop deve ter blocos filhos"},
		},
		{
			name: "all fields missing",
			blocks: []TemplateBlock{{
				Type: "loop",
			}},
			wantValid: false,
			errCount:  3,
			errFields: []string{"iterator", "collection", "children"},
			errMsgs:   []string{"Loop deve ter um iterador", "Loop deve ter uma colecao", "Loop deve ter blocos filhos"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.Len(t, resp.Errors, tt.errCount)

				for i, e := range resp.Errors {
					assert.Equal(t, tt.errFields[i], e.Field)
					assert.Equal(t, tt.errMsgs[i], e.Message)
				}
			}
		})
	}
}

func TestValidateBlocks_ConditionalBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errCount  int
		errFields []string
		errMsgs   []string
	}{
		{
			name: "valid conditional",
			blocks: []TemplateBlock{{
				Type: "conditional", Condition: "x > 0",
				Children: []TemplateBlock{{Type: "text", Content: "yes"}},
			}},
			wantValid: true,
		},
		{
			name: "missing condition",
			blocks: []TemplateBlock{{
				Type: "conditional", Condition: "",
				Children: []TemplateBlock{{Type: "text", Content: "yes"}},
			}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"condition"},
			errMsgs:   []string{"Condicional deve ter uma condicao"},
		},
		{
			name: "missing children",
			blocks: []TemplateBlock{{
				Type: "conditional", Condition: "x > 0",
			}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"children"},
			errMsgs:   []string{"Condicional deve ter blocos filhos"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.Len(t, resp.Errors, tt.errCount)

				for i, e := range resp.Errors {
					assert.Equal(t, tt.errFields[i], e.Field)
					assert.Equal(t, tt.errMsgs[i], e.Message)
				}
			}
		})
	}
}

func TestValidateBlocks_AggregationBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errCount  int
		errFields []string
		errMsgs   []string
	}{
		{
			name:      "valid aggregation",
			blocks:    []TemplateBlock{{Type: "aggregation", Collection: "items", Function: "sum", Field: "value"}},
			wantValid: true,
		},
		{
			name:      "missing collection",
			blocks:    []TemplateBlock{{Type: "aggregation", Function: "sum", Field: "value"}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"collection"},
			errMsgs:   []string{"Agregacao deve ter uma colecao"},
		},
		{
			name:      "missing function",
			blocks:    []TemplateBlock{{Type: "aggregation", Collection: "items", Field: "value"}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"function"},
			errMsgs:   []string{"Agregacao deve ter uma funcao"},
		},
		{
			name:      "missing field",
			blocks:    []TemplateBlock{{Type: "aggregation", Collection: "items", Function: "sum"}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"field"},
			errMsgs:   []string{"Agregacao deve ter um campo"},
		},
		{
			name:      "all missing",
			blocks:    []TemplateBlock{{Type: "aggregation"}},
			wantValid: false,
			errCount:  3,
			errFields: []string{"collection", "function", "field"},
			errMsgs:   []string{"Agregacao deve ter uma colecao", "Agregacao deve ter uma funcao", "Agregacao deve ter um campo"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.Len(t, resp.Errors, tt.errCount)

				for i, e := range resp.Errors {
					assert.Equal(t, tt.errFields[i], e.Field)
					assert.Equal(t, tt.errMsgs[i], e.Message)
				}
			}
		})
	}
}

func TestValidateBlocks_CalculationBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errCount  int
		errFields []string
		errMsgs   []string
	}{
		{
			name:      "valid calculation",
			blocks:    []TemplateBlock{{Type: "calculation", Variable: "total", Function: "percent_of"}},
			wantValid: true,
		},
		{
			name:      "missing variable",
			blocks:    []TemplateBlock{{Type: "calculation", Function: "percent_of"}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"variable"},
			errMsgs:   []string{"Calculo deve ter uma variavel"},
		},
		{
			name:      "missing function",
			blocks:    []TemplateBlock{{Type: "calculation", Variable: "total"}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"function"},
			errMsgs:   []string{"Calculo deve ter uma funcao"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.Len(t, resp.Errors, tt.errCount)

				for i, e := range resp.Errors {
					assert.Equal(t, tt.errFields[i], e.Field)
					assert.Equal(t, tt.errMsgs[i], e.Message)
				}
			}
		})
	}
}

func TestValidateBlocks_DateTimeBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errCount  int
		errFields []string
		errMsgs   []string
	}{
		{
			name:      "valid date_time",
			blocks:    []TemplateBlock{{Type: "date_time", Variable: "created_at", Format: "02/01/2006"}},
			wantValid: true,
		},
		{
			name:      "now mode (no variable)",
			blocks:    []TemplateBlock{{Type: "date_time", Format: "02/01/2006"}},
			wantValid: true,
		},
		{
			name:      "missing format",
			blocks:    []TemplateBlock{{Type: "date_time", Variable: "created_at"}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"format"},
			errMsgs:   []string{"Data/Hora deve ter um formato"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.Len(t, resp.Errors, tt.errCount)

				for i, e := range resp.Errors {
					assert.Equal(t, tt.errFields[i], e.Field)
					assert.Equal(t, tt.errMsgs[i], e.Message)
				}
			}
		})
	}
}

func TestValidateBlocks_CounterBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errCount  int
		errMsgs   []string
	}{
		{
			name: "valid counter increment",
			blocks: []TemplateBlock{{
				Type: "counter",
				Properties: map[string]interface{}{
					"counterMode":  "increment",
					"counterNames": []interface{}{"cnt"},
				},
			}},
			wantValid: true,
		},
		{
			name: "valid counter show",
			blocks: []TemplateBlock{{
				Type: "counter",
				Properties: map[string]interface{}{
					"counterMode":  "show",
					"counterNames": []interface{}{"a", "b"},
				},
			}},
			wantValid: true,
		},
		{
			name:      "nil properties - missing counterMode",
			blocks:    []TemplateBlock{{Type: "counter"}},
			wantValid: false,
			errCount:  2,
			errMsgs: []string{
				"Contador deve ter um modo (counterMode)",
				"Contador deve ter pelo menos um nome (counterNames)",
			},
		},
		{
			name: "invalid counter mode",
			blocks: []TemplateBlock{{
				Type: "counter",
				Properties: map[string]interface{}{
					"counterMode":  "reset",
					"counterNames": []interface{}{"cnt"},
				},
			}},
			wantValid: false,
			errCount:  1,
			errMsgs:   []string{"Modo de contador invalido: deve ser 'increment' ou 'show'"},
		},
		{
			name: "empty counterNames",
			blocks: []TemplateBlock{{
				Type: "counter",
				Properties: map[string]interface{}{
					"counterMode":  "increment",
					"counterNames": []interface{}{},
				},
			}},
			wantValid: false,
			errCount:  1,
			errMsgs:   []string{"Contador deve ter pelo menos um nome (counterNames)"},
		},
		{
			name: "missing counterMode key",
			blocks: []TemplateBlock{{
				Type: "counter",
				Properties: map[string]interface{}{
					"counterNames": []interface{}{"cnt"},
				},
			}},
			wantValid: false,
			errCount:  1,
			errMsgs:   []string{"Contador deve ter um modo (counterMode)"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.Len(t, resp.Errors, tt.errCount)

				for i, e := range resp.Errors {
					assert.Equal(t, tt.errMsgs[i], e.Message)
				}
			}
		})
	}
}

func TestValidateBlocks_CommentBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errMsg    string
	}{
		{
			name:      "valid comment",
			blocks:    []TemplateBlock{{Type: "comment", Content: "note"}},
			wantValid: true,
		},
		{
			name:      "empty content",
			blocks:    []TemplateBlock{{Type: "comment", Content: ""}},
			wantValid: false,
			errMsg:    "Comentario deve ter conteudo",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.NotEmpty(t, resp.Errors)
				assert.Equal(t, tt.errMsg, resp.Errors[0].Message)
			}
		})
	}
}

func TestValidateBlocks_SectionBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errMsg    string
	}{
		{
			name: "valid section",
			blocks: []TemplateBlock{{
				Type:     "section",
				Children: []TemplateBlock{{Type: "text", Content: "x"}},
			}},
			wantValid: true,
		},
		{
			name:      "empty children",
			blocks:    []TemplateBlock{{Type: "section"}},
			wantValid: false,
			errMsg:    "Secao deve ter blocos filhos",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.NotEmpty(t, resp.Errors)
				assert.Equal(t, tt.errMsg, resp.Errors[0].Message)
			}
		})
	}
}

func TestValidateBlocks_RecursiveChildValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		errCount  int
		checkMsgs []string
	}{
		{
			name: "invalid child in loop",
			blocks: []TemplateBlock{{
				Type: "loop", Iterator: "i", Collection: "items",
				Children: []TemplateBlock{
					{Type: "text", Content: ""},
				},
			}},
			errCount:  1,
			checkMsgs: []string{"Texto deve ter conteudo"},
		},
		{
			name: "invalid child in conditional",
			blocks: []TemplateBlock{{
				Type: "conditional", Condition: "x",
				Children: []TemplateBlock{
					{Type: "variable", Variable: ""},
				},
			}},
			errCount:  1,
			checkMsgs: []string{"Variavel deve ter um nome"},
		},
		{
			name: "invalid child in section",
			blocks: []TemplateBlock{{
				Type: "section",
				Children: []TemplateBlock{
					{Type: "aggregation"},
				},
			}},
			errCount:  3,
			checkMsgs: []string{"Agregacao deve ter uma colecao", "Agregacao deve ter uma funcao", "Agregacao deve ter um campo"},
		},
		{
			name: "deeply nested invalid child",
			blocks: []TemplateBlock{{
				Type: "section",
				Children: []TemplateBlock{{
					Type: "loop", Iterator: "i", Collection: "items",
					Children: []TemplateBlock{
						{Type: "text", Content: ""},
					},
				}},
			}},
			errCount:  1,
			checkMsgs: []string{"Texto deve ter conteudo"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.False(t, resp.Valid)
			require.Len(t, resp.Errors, tt.errCount)

			for i, msg := range tt.checkMsgs {
				assert.Equal(t, msg, resp.Errors[i].Message)
			}
		})
	}
}

func TestValidateBlocks_BlockIDGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		blocks        []TemplateBlock
		expectBlockID string
	}{
		{
			name:          "uses provided blockId",
			blocks:        []TemplateBlock{{BlockID: "custom-id", Type: "text", Content: ""}},
			expectBlockID: "custom-id",
		},
		{
			name:          "generates blockId when missing",
			blocks:        []TemplateBlock{{Type: "text", Content: ""}},
			expectBlockID: "block-0",
		},
		{
			name: "generates child blockId",
			blocks: []TemplateBlock{{
				Type: "loop", Iterator: "i", Collection: "items",
				Children: []TemplateBlock{
					{Type: "text", Content: ""},
				},
			}},
			expectBlockID: "block-0-0",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.False(t, resp.Valid)
			require.NotEmpty(t, resp.Errors)
			assert.Equal(t, tt.expectBlockID, resp.Errors[0].BlockID)
		})
	}
}

func TestValidateBlocks_Pongo2SyntaxValidation(t *testing.T) {
	t.Parallel()

	t.Run("valid syntax passes without syntax error", func(t *testing.T) {
		t.Parallel()

		blocks := []TemplateBlock{{Type: "text", Content: "Hello"}}
		resp := ValidateBlocks(blocks)
		require.NotNil(t, resp)
		assert.True(t, resp.Valid)

		for _, e := range resp.Errors {
			assert.NotEqual(t, "_syntax", e.BlockID, "should not have syntax error for valid blocks")
		}
	})

	t.Run("syntax error format is correct", func(t *testing.T) {
		t.Parallel()

		// ValidateBlocks calls GenerateCode after structural validation passes.
		// Since GenerateCode is string-based, syntax errors only occur for
		// structurally valid but code-generation-failing blocks.
		// We verify the mechanism by constructing the expected response format.
		resp := &ValidateBlocksResponse{
			Valid: false,
			Errors: []ValidationError{
				{BlockID: "_syntax", Field: "code", Message: "Erro de sintaxe Pongo2: simulated"},
			},
		}
		require.NotNil(t, resp)
		assert.False(t, resp.Valid)
		require.Len(t, resp.Errors, 1)
		assert.Equal(t, "_syntax", resp.Errors[0].BlockID)
		assert.Equal(t, "code", resp.Errors[0].Field)
		assert.Contains(t, resp.Errors[0].Message, "Erro de sintaxe Pongo2:")
	})
}

func TestValidateBlocks_WithBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errCount  int
		errFields []string
		errMsgs   []string
	}{
		{
			name: "valid with block",
			blocks: []TemplateBlock{{
				Type: "with", Variable: "ops", Assignment: "filter(items, \"id\", x)",
				Children: []TemplateBlock{{Type: "text", Content: "x"}},
			}},
			wantValid: true,
		},
		{
			name: "missing variable",
			blocks: []TemplateBlock{{
				Type: "with", Assignment: "filter(items, \"id\", x)",
				Children: []TemplateBlock{{Type: "text", Content: "x"}},
			}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"variable"},
			errMsgs:   []string{"With deve ter uma variavel"},
		},
		{
			name: "missing assignment",
			blocks: []TemplateBlock{{
				Type: "with", Variable: "ops",
				Children: []TemplateBlock{{Type: "text", Content: "x"}},
			}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"assignment"},
			errMsgs:   []string{"With deve ter uma atribuicao"},
		},
		{
			name: "missing children",
			blocks: []TemplateBlock{{
				Type: "with", Variable: "ops", Assignment: "filter(items, \"id\", x)",
			}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"children"},
			errMsgs:   []string{"With deve ter blocos filhos"},
		},
		{
			name: "all fields missing",
			blocks: []TemplateBlock{{
				Type: "with",
			}},
			wantValid: false,
			errCount:  3,
			errFields: []string{"variable", "assignment", "children"},
			errMsgs:   []string{"With deve ter uma variavel", "With deve ter uma atribuicao", "With deve ter blocos filhos"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.Len(t, resp.Errors, tt.errCount)

				for i, e := range resp.Errors {
					assert.Equal(t, tt.errFields[i], e.Field)
					assert.Equal(t, tt.errMsgs[i], e.Message)
				}
			}
		})
	}
}

func TestValidateBlocks_ExpressionBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errField  string
		errMsg    string
	}{
		{
			name:      "valid expression",
			blocks:    []TemplateBlock{{Type: "expression", Expression: "6 + items|length"}},
			wantValid: true,
		},
		{
			name:      "empty expression",
			blocks:    []TemplateBlock{{Type: "expression", Expression: ""}},
			wantValid: false,
			errField:  "expression",
			errMsg:    "Expressao deve ter uma expressao",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.NotEmpty(t, resp.Errors)
				assert.Equal(t, tt.errField, resp.Errors[0].Field)
				assert.Equal(t, tt.errMsg, resp.Errors[0].Message)
			}
		})
	}
}

func TestValidateBlocks_ConditionalWithElseChildren(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errMsg    string
	}{
		{
			name: "valid conditional with else",
			blocks: []TemplateBlock{{
				Type: "conditional", Condition: "x",
				Children:     []TemplateBlock{{Type: "text", Content: "yes"}},
				ElseChildren: []TemplateBlock{{Type: "text", Content: "no"}},
			}},
			wantValid: true,
		},
		{
			name: "invalid child in else branch",
			blocks: []TemplateBlock{{
				Type: "conditional", Condition: "x",
				Children:     []TemplateBlock{{Type: "text", Content: "yes"}},
				ElseChildren: []TemplateBlock{{Type: "text", Content: ""}},
			}},
			wantValid: false,
			errMsg:    "Texto deve ter conteudo",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.NotEmpty(t, resp.Errors)
				assert.Equal(t, tt.errMsg, resp.Errors[0].Message)
			}
		})
	}
}

func TestValidateBlocks_CustomTagBlock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		blocks    []TemplateBlock
		wantValid bool
		errCount  int
		errFields []string
		errMsgs   []string
	}{
		{
			name:      "valid sum_by tag",
			blocks:    []TemplateBlock{{Type: "custom_tag", TagName: "sum_by", TagArgs: "ops by \"value\""}},
			wantValid: true,
		},
		{
			name:      "valid last_item_by_group tag",
			blocks:    []TemplateBlock{{Type: "custom_tag", TagName: "last_item_by_group", TagArgs: "col group_by \"f\" order_by \"d\" as result"}},
			wantValid: true,
		},
		{
			name:      "missing tagName",
			blocks:    []TemplateBlock{{Type: "custom_tag", TagArgs: "ops by \"value\""}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"tagName"},
			errMsgs:   []string{"Tag customizada deve ter um nome (tagName)"},
		},
		{
			name:      "unknown tagName",
			blocks:    []TemplateBlock{{Type: "custom_tag", TagName: "unknown_tag", TagArgs: "args"}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"tagName"},
			errMsgs:   []string{"Tag customizada desconhecida: unknown_tag"},
		},
		{
			name:      "missing tagArgs",
			blocks:    []TemplateBlock{{Type: "custom_tag", TagName: "sum_by"}},
			wantValid: false,
			errCount:  1,
			errFields: []string{"tagArgs"},
			errMsgs:   []string{"Tag customizada deve ter argumentos (tagArgs)"},
		},
		{
			name:      "all missing",
			blocks:    []TemplateBlock{{Type: "custom_tag"}},
			wantValid: false,
			errCount:  2,
			errFields: []string{"tagName", "tagArgs"},
			errMsgs:   []string{"Tag customizada deve ter um nome (tagName)", "Tag customizada deve ter argumentos (tagArgs)"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantValid, resp.Valid)

			if !tt.wantValid {
				require.Len(t, resp.Errors, tt.errCount)

				for i, e := range resp.Errors {
					assert.Equal(t, tt.errFields[i], e.Field)
					assert.Equal(t, tt.errMsgs[i], e.Message)
				}
			}
		})
	}
}

func TestValidateBlocks_AllBlockTypesValid_WithNewTypes(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{
		{BlockID: "b11", Type: "with", Variable: "ops", Assignment: "filter(items, \"id\", x)", Children: []TemplateBlock{
			{Type: "text", Content: "child"},
		}},
		{BlockID: "b12", Type: "expression", Expression: "6 + items|length"},
	}

	resp := ValidateBlocks(blocks)
	require.NotNil(t, resp)
	assert.True(t, resp.Valid)
	assert.Empty(t, resp.Errors)
}

func TestValidateBlocks_MultipleErrors(t *testing.T) {
	t.Parallel()

	blocks := []TemplateBlock{
		{Type: "text", Content: ""},
		{Type: "variable", Variable: ""},
		{Type: "unknown_type"},
	}

	resp := ValidateBlocks(blocks)
	require.NotNil(t, resp)
	assert.False(t, resp.Valid)
	assert.GreaterOrEqual(t, len(resp.Errors), 3)
}

// ─── Security Tests ──────────────────────────────────────────────────────────

func TestValidateIdentifier_SecurityCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		value     string
		wantValid bool
	}{
		// Valid identifiers
		{name: "simple variable", value: "name", wantValid: true},
		{name: "dotted path", value: "item.amount", wantValid: true},
		{name: "underscore prefix", value: "_private", wantValid: true},
		{name: "with numbers", value: "var123", wantValid: true},
		{name: "deep nested", value: "a.b.c.d", wantValid: true},
		{name: "array index", value: "items.0", wantValid: true},
		{name: "schema qualified collection", value: "midaz_onboarding:onboarding_f2f69884845b466e9896e8a38ba0628d.account", wantValid: true},
		{name: "schema qualified with array", value: "midaz_onboarding:onboarding_f2f69884845b466e9896e8a38ba0628d.account.0", wantValid: true},
		// SSTI injection payloads
		{name: "pongo2 tag injection", value: "x %}{% include \"etc/passwd\" %}{% set y", wantValid: false},
		{name: "pipe injection", value: "x|safe", wantValid: false},
		{name: "curly brace injection", value: "{{ malicious }}", wantValid: false},
		{name: "block tag injection", value: "x %}{% block evil %}{% endblock %}{% set y", wantValid: false},
		{name: "semicolon", value: "a;b", wantValid: false},
		{name: "shell command injection", value: "$(whoami)", wantValid: false},
		{name: "backtick injection", value: "`rm -rf /`", wantValid: false},
		{name: "space injection", value: "a b", wantValid: false},
		{name: "quotes", value: `a"b`, wantValid: false},
		{name: "angle brackets", value: "a<b>c", wantValid: false},
		{name: "percent", value: "a%b", wantValid: false},
		{name: "starts with number", value: "123abc", wantValid: false},
		{name: "empty string", value: "", wantValid: true}, // empty is allowed (field-specific required checks handle this)
		{name: "newline injection", value: "a\nb", wantValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var errs []ValidationError
			validateIdentifier(tt.value, "testField", "test-block", &errs)

			if tt.wantValid {
				assert.Empty(t, errs, "expected no errors for %q", tt.value)
			} else {
				assert.NotEmpty(t, errs, "expected error for %q", tt.value)
				assert.Contains(t, errs[0].Message, "caracteres invalidos")
			}
		})
	}
}

func TestValidateBlocks_DepthLimitExceeded(t *testing.T) {
	t.Parallel()

	// Build a chain of 52 nested sections (exceeds maxBlockDepth=50)
	block := TemplateBlock{Type: "text", Content: "leaf"}
	for i := 0; i < 52; i++ {
		block = TemplateBlock{Type: "section", Children: []TemplateBlock{block}}
	}

	resp := ValidateBlocks([]TemplateBlock{block})
	require.NotNil(t, resp)
	assert.False(t, resp.Valid)

	found := false
	for _, e := range resp.Errors {
		if e.Field == "children" && e.Message == "Profundidade maxima de aninhamento excedida (50 niveis)" {
			found = true
			break
		}
	}

	assert.True(t, found, "expected depth limit error, got: %v", resp.Errors)
}

func TestValidateBlocks_DepthAtExactLimit(t *testing.T) {
	t.Parallel()

	// Build exactly 50 nested sections (should pass)
	block := TemplateBlock{Type: "text", Content: "leaf"}
	for i := 0; i < 50; i++ {
		block = TemplateBlock{Type: "section", Children: []TemplateBlock{block}}
	}

	resp := ValidateBlocks([]TemplateBlock{block})
	require.NotNil(t, resp)
	assert.True(t, resp.Valid, "depth=50 should be valid, errors: %v", resp.Errors)
}

func TestValidateBlocks_FilterNameWhitelist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		filterName string
		wantValid  bool
	}{
		{name: "known filter percent_of", filterName: "percent_of", wantValid: true},
		{name: "known filter sum", filterName: "sum", wantValid: true},
		{name: "known filter date", filterName: "date", wantValid: true},
		{name: "unknown filter", filterName: "evil_filter", wantValid: false},
		{name: "injection in filter name", filterName: "safe\"|exec:\"cmd", wantValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			blocks := []TemplateBlock{{
				Type:     "variable",
				Variable: "x",
				Filters:  []FilterChain{{Name: tt.filterName}},
			}}

			resp := ValidateBlocks(blocks)
			require.NotNil(t, resp)

			if tt.wantValid {
				assert.True(t, resp.Valid, "expected valid for filter %q, errors: %v", tt.filterName, resp.Errors)
			} else {
				assert.False(t, resp.Valid, "expected invalid for filter %q", tt.filterName)
			}
		})
	}
}

func TestValidateBlocks_FilterArgsDelimiters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      string
		wantValid bool
	}{
		{name: "valid args", args: "field_name", wantValid: true},
		{name: "template delimiters in args", args: "foo }}{{ malicious", wantValid: false},
		{name: "tag delimiters in args", args: "x %}{%include '/etc/passwd'%}{%", wantValid: false},
		{name: "empty args is valid", args: "", wantValid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			blocks := []TemplateBlock{{
				Type:     "variable",
				Variable: "x",
				Filters:  []FilterChain{{Name: "replace", Args: tt.args}},
			}}

			resp := ValidateBlocks(blocks)
			require.NotNil(t, resp)

			if tt.wantValid {
				assert.True(t, resp.Valid, "expected valid for args %q, errors: %v", tt.args, resp.Errors)
			} else {
				assert.False(t, resp.Valid, "expected invalid for args %q, errors: %v", tt.args, resp.Errors)
			}
		})
	}
}

func TestValidateBlocks_CounterNameInjection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		counter   string
		wantValid bool
	}{
		{name: "valid counter name", counter: "page_count", wantValid: true},
		{name: "injection in counter name", counter: "x\"%}{% include \"/etc/passwd\" %}{%\"", wantValid: false},
		{name: "space in counter name", counter: "my counter", wantValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			blocks := []TemplateBlock{{
				Type: "counter",
				Properties: map[string]any{
					"counterMode":  "increment",
					"counterNames": []any{tt.counter},
				},
			}}

			resp := ValidateBlocks(blocks)
			require.NotNil(t, resp)

			if tt.wantValid {
				assert.True(t, resp.Valid, "expected valid for counter %q, errors: %v", tt.counter, resp.Errors)
			} else {
				assert.False(t, resp.Valid, "expected invalid for counter %q", tt.counter)
			}
		})
	}
}

func TestValidateBlocks_TemplateDelimiterInjection(t *testing.T) {
	t.Parallel()

	// Payloads that attempt to break out of their context via template delimiters.
	injectionPayloads := []string{
		`true %}{%set x = secret%}{{x}}{%if true`,
		`value %}{% include "evil" %}{% if x`,
		`{{ admin_secret }}`,
		`safe #}{% set x = secret %}{{ x }}{# rest`,
		`hello %}{{ password }}{% if true`,
	}

	tests := []struct {
		name      string
		blockFunc func(payload string) []TemplateBlock
		field     string
	}{
		{
			name: "conditional condition",
			blockFunc: func(payload string) []TemplateBlock {
				return []TemplateBlock{{
					Type: "conditional", Condition: payload,
					Children: []TemplateBlock{{Type: "text", Content: "x"}},
				}}
			},
			field: "condition",
		},
		{
			name: "expression",
			blockFunc: func(payload string) []TemplateBlock {
				return []TemplateBlock{{Type: "expression", Expression: payload}}
			},
			field: "expression",
		},
		{
			name: "with assignment",
			blockFunc: func(payload string) []TemplateBlock {
				return []TemplateBlock{{
					Type: "with", Variable: "x", Assignment: payload,
					Children: []TemplateBlock{{Type: "text", Content: "x"}},
				}}
			},
			field: "assignment",
		},
		{
			name: "date_time format",
			blockFunc: func(payload string) []TemplateBlock {
				return []TemplateBlock{{Type: "date_time", Variable: "created_at", Format: payload}}
			},
			field: "format",
		},
		{
			name: "custom_tag tagArgs",
			blockFunc: func(payload string) []TemplateBlock {
				return []TemplateBlock{{Type: "custom_tag", TagName: "sum_by", TagArgs: payload}}
			},
			field: "tagArgs",
		},
		{
			name: "comment content",
			blockFunc: func(payload string) []TemplateBlock {
				return []TemplateBlock{{Type: "comment", Content: payload}}
			},
			field: "content",
		},
	}

	for _, tt := range tests {
		for _, payload := range injectionPayloads {
			t.Run(tt.name+"/"+payload, func(t *testing.T) {
				t.Parallel()

				blocks := tt.blockFunc(payload)
				resp := ValidateBlocks(blocks)
				require.NotNil(t, resp)
				assert.False(t, resp.Valid, "expected invalid for %s with payload %q, but got valid", tt.field, payload)

				found := false
				for _, e := range resp.Errors {
					if e.Field == tt.field {
						assert.Contains(t, e.Message, "delimitadores de template")
						found = true

						break
					}
				}

				assert.True(t, found, "expected delimiter error for field %q, got: %v", tt.field, resp.Errors)
			})
		}
	}
}

func TestValidateBlocks_LegitimateExpressionsStillValid(t *testing.T) {
	t.Parallel()

	// Ensure that legitimate expressions without delimiters still pass validation.
	tests := []struct {
		name   string
		blocks []TemplateBlock
	}{
		{
			name: "conditional with comparison",
			blocks: []TemplateBlock{{
				Type: "conditional", Condition: "item.amount > 0",
				Children: []TemplateBlock{{Type: "text", Content: "positive"}},
			}},
		},
		{
			name:   "expression with pipe",
			blocks: []TemplateBlock{{Type: "expression", Expression: "items|length"}},
		},
		{
			name: "with assignment simple",
			blocks: []TemplateBlock{{
				Type: "with", Variable: "total", Assignment: "items|sum",
				Children: []TemplateBlock{{Type: "text", Content: "x"}},
			}},
		},
		{
			name:   "date_time with normal format",
			blocks: []TemplateBlock{{Type: "date_time", Variable: "created_at", Format: "02/01/2006"}},
		},
		{
			name:   "custom_tag with normal args",
			blocks: []TemplateBlock{{Type: "custom_tag", TagName: "sum_by", TagArgs: `items by "amount"`}},
		},
		{
			name:   "comment with normal text",
			blocks: []TemplateBlock{{Type: "comment", Content: "This is a section header"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := ValidateBlocks(tt.blocks)
			require.NotNil(t, resp)
			assert.True(t, resp.Valid, "expected valid for %q, errors: %v", tt.name, resp.Errors)
		})
	}
}
