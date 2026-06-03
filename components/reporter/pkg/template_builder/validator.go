// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template_builder

import (
	"fmt"
	"regexp"
	"strings"
)

const maxBlockDepth = 50

// validIdentifier allows alphanumeric characters, dots, underscores, colons (for datasource:schema
// qualified names like midaz_onboarding:onboarding_org_xxx.account), and array index access.
var validIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.:]*(\.\d+)?$`)

// templateDelimiters are Pongo2 delimiters that must not appear in free-form expression fields.
// These fields are already placed inside delimiters by the code generator, so user input
// containing delimiters indicates an injection attempt (SSTI).
var templateDelimiters = []string{"{%", "%}", "{{", "}}", "{#", "#}"}

// containsTemplateDelimiters returns true if the value contains any Pongo2 template delimiters.
func containsTemplateDelimiters(value string) bool {
	for _, d := range templateDelimiters {
		if strings.Contains(value, d) {
			return true
		}
	}

	return false
}

// validateNoDelimiters checks that a free-form field does not contain template delimiters.
func validateNoDelimiters(value, fieldName, blockID string, errs *[]ValidationError) {
	if containsTemplateDelimiters(value) {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   fieldName,
			Message: fmt.Sprintf("%s contem delimitadores de template nao permitidos ({%%, %%}, {{, }})", fieldName),
		})
	}
}

// validBlockTypes is the set of all recognized block types.
var validBlockTypes = map[string]bool{
	"text":        true,
	"variable":    true,
	"loop":        true,
	"conditional": true,
	"aggregation": true,
	"calculation": true,
	"date_time":   true,
	"counter":     true,
	"comment":     true,
	"section":     true,
	"with":        true,
	"expression":  true,
	"custom_tag":  true,
}

// ValidateBlocks validates a slice of TemplateBlock and returns detailed errors with block identification.
//
//nolint:cyclop // validation dispatch requires multiple branches per block type
func ValidateBlocks(blocks []TemplateBlock) *ValidateBlocksResponse {
	var errs []ValidationError

	validateBlocksRecursive(blocks, "", 0, &errs)

	// If structural validation passes, attempt Pongo2 syntax validation.
	if len(errs) == 0 {
		if _, err := GenerateCode(blocks, ""); err != nil {
			errs = append(errs, ValidationError{
				BlockID: "_syntax",
				Field:   "code",
				Message: fmt.Sprintf("Erro de sintaxe Pongo2: %v", err),
			})
		}
	}

	return &ValidateBlocksResponse{
		Valid:  len(errs) == 0,
		Errors: errs,
	}
}

// validateBlocksRecursive validates each block in the slice, generating BlockIDs as needed.
func validateBlocksRecursive(blocks []TemplateBlock, parentPrefix string, depth int, errs *[]ValidationError) {
	if depth > maxBlockDepth {
		*errs = append(*errs, ValidationError{
			BlockID: parentPrefix,
			Field:   "children",
			Message: fmt.Sprintf("Profundidade maxima de aninhamento excedida (%d niveis)", maxBlockDepth),
		})

		return
	}

	for i := range blocks {
		block := &blocks[i]
		blockID := block.BlockID

		if blockID == "" {
			if parentPrefix == "" {
				blockID = fmt.Sprintf("block-%d", i)
			} else {
				blockID = fmt.Sprintf("%s-%d", parentPrefix, i)
			}
		}

		validateSingleBlock(block, blockID, depth, errs)
	}
}

// validateIdentifier checks if a value is a safe Pongo2 identifier (alphanumeric, dots, underscores).
func validateIdentifier(value, fieldName, blockID string, errs *[]ValidationError) {
	if value != "" && !validIdentifier.MatchString(value) {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   fieldName,
			Message: fmt.Sprintf("%s contem caracteres invalidos", fieldName),
		})
	}
}

// validateSingleBlock validates a single block and appends errors to the slice.
//
//nolint:cyclop // block type dispatch requires multiple branches
func validateSingleBlock(block *TemplateBlock, blockID string, depth int, errs *[]ValidationError) {
	if !validBlockTypes[block.Type] {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "type",
			Message: fmt.Sprintf("Tipo de bloco invalido: %s", block.Type),
		})

		return
	}

	switch block.Type {
	case "text":
		validateTextBlock(block, blockID, errs)
	case "variable":
		validateVariableBlock(block, blockID, errs)
	case "loop":
		validateLoopBlock(block, blockID, depth, errs)
	case "conditional":
		validateConditionalBlock(block, blockID, depth, errs)
	case "aggregation":
		validateAggregationBlock(block, blockID, errs)
	case "calculation":
		validateCalculationBlock(block, blockID, errs)
	case "date_time":
		validateDateTimeBlock(block, blockID, errs)
	case "counter":
		validateCounterBlock(block, blockID, errs)
	case "comment":
		validateCommentBlock(block, blockID, errs)
	case "section":
		validateSectionBlock(block, blockID, depth, errs)
	case "with":
		validateWithBlock(block, blockID, depth, errs)
	case "expression":
		validateExpressionBlock(block, blockID, errs)
	case "custom_tag":
		validateCustomTagBlock(block, blockID, errs)
	}
}

func validateTextBlock(block *TemplateBlock, blockID string, errs *[]ValidationError) {
	if block.Content == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "content",
			Message: "Texto deve ter conteudo",
		})
	}
}

func validateVariableBlock(block *TemplateBlock, blockID string, errs *[]ValidationError) {
	if block.Variable == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "variable",
			Message: "Variavel deve ter um nome",
		})
	} else {
		validateIdentifier(block.Variable, "variable", blockID, errs)
	}

	for i, f := range block.Filters {
		filterID := fmt.Sprintf("%s-filter-%d", blockID, i)

		if f.Name == "" {
			*errs = append(*errs, ValidationError{
				BlockID: filterID,
				Field:   "name",
				Message: "Filtro deve ter um nome",
			})
		} else if !validFilterNames[f.Name] {
			*errs = append(*errs, ValidationError{
				BlockID: filterID,
				Field:   "name",
				Message: fmt.Sprintf("Filtro desconhecido: %s", f.Name),
			})
		}

		if f.Args != "" {
			validateNoDelimiters(f.Args, "args", filterID, errs)
		}
	}
}

func validateLoopBlock(block *TemplateBlock, blockID string, depth int, errs *[]ValidationError) {
	if block.Iterator == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "iterator",
			Message: "Loop deve ter um iterador",
		})
	} else {
		validateIdentifier(block.Iterator, "iterator", blockID, errs)
	}

	if block.Collection == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "collection",
			Message: "Loop deve ter uma colecao",
		})
	} else {
		validateIdentifier(block.Collection, "collection", blockID, errs)
	}

	if len(block.Children) == 0 {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "children",
			Message: "Loop deve ter blocos filhos",
		})
	} else {
		validateBlocksRecursive(block.Children, blockID, depth+1, errs)
	}
}

//nolint:misspell // "Condicional" is Portuguese, not a misspelling of "Conditional"
func validateConditionalBlock(block *TemplateBlock, blockID string, depth int, errs *[]ValidationError) {
	if block.Condition == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "condition",
			Message: "Condicional deve ter uma condicao", //nolint:misspell // Portuguese
		})
	} else {
		validateNoDelimiters(block.Condition, "condition", blockID, errs)
	}

	if len(block.Children) == 0 {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "children",
			Message: "Condicional deve ter blocos filhos", //nolint:misspell // Portuguese
		})
	} else {
		validateBlocksRecursive(block.Children, blockID, depth+1, errs)
	}

	for i, elif := range block.ElifBranches {
		elifID := fmt.Sprintf("%s-elif-%d", blockID, i)

		if elif.Condition == "" {
			*errs = append(*errs, ValidationError{
				BlockID: elifID,
				Field:   "condition",
				Message: "Elif deve ter uma condicao",
			})
		} else {
			validateNoDelimiters(elif.Condition, "condition", elifID, errs)
		}

		if len(elif.Children) == 0 {
			*errs = append(*errs, ValidationError{
				BlockID: elifID,
				Field:   "children",
				Message: "Elif deve ter blocos filhos",
			})
		} else {
			validateBlocksRecursive(elif.Children, elifID, depth+1, errs)
		}
	}

	if len(block.ElseChildren) > 0 {
		validateBlocksRecursive(block.ElseChildren, blockID+"-else", depth+1, errs)
	}
}

func validateAggregationBlock(block *TemplateBlock, blockID string, errs *[]ValidationError) {
	if block.Collection == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "collection",
			Message: "Agregacao deve ter uma colecao",
		})
	} else {
		validateIdentifier(block.Collection, "collection", blockID, errs)
	}

	if block.Function == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "function",
			Message: "Agregacao deve ter uma funcao",
		})
	} else {
		validateIdentifier(block.Function, "function", blockID, errs)
	}

	if block.Field == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "field",
			Message: "Agregacao deve ter um campo",
		})
	} else {
		validateIdentifier(block.Field, "field", blockID, errs)
	}
}

func validateCalculationBlock(block *TemplateBlock, blockID string, errs *[]ValidationError) {
	if block.Variable == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "variable",
			Message: "Calculo deve ter uma variavel",
		})
	} else {
		validateIdentifier(block.Variable, "variable", blockID, errs)
	}

	if block.Function == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "function",
			Message: "Calculo deve ter uma funcao",
		})
	} else {
		validateIdentifier(block.Function, "function", blockID, errs)
	}
}

func validateDateTimeBlock(block *TemplateBlock, blockID string, errs *[]ValidationError) {
	// Variable is optional: when omitted, the block generates the current date (now mode).
	if block.Variable != "" {
		validateIdentifier(block.Variable, "variable", blockID, errs)
	}

	if block.Format == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "format",
			Message: "Data/Hora deve ter um formato",
		})
	} else {
		validateNoDelimiters(block.Format, "format", blockID, errs)
	}
}

func validateCounterBlock(block *TemplateBlock, blockID string, errs *[]ValidationError) {
	mode, modeOK := "", false

	var namesRaw []any

	if block.Properties != nil {
		mode, modeOK = block.Properties["counterMode"].(string)
		namesRaw, _ = block.Properties["counterNames"].([]any)
	}

	if !modeOK || mode == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "counterMode",
			Message: "Contador deve ter um modo (counterMode)",
		})
	} else if mode != "increment" && mode != "show" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "counterMode",
			Message: "Modo de contador invalido: deve ser 'increment' ou 'show'",
		})
	}

	names := make([]string, 0, len(namesRaw))

	for _, n := range namesRaw {
		if s, ok := n.(string); ok {
			names = append(names, s)
		}
	}

	if len(names) == 0 {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "counterNames",
			Message: "Contador deve ter pelo menos um nome (counterNames)",
		})
	} else {
		for _, n := range names {
			validateIdentifier(n, "counterNames", blockID, errs)
		}
	}
}

func validateCommentBlock(block *TemplateBlock, blockID string, errs *[]ValidationError) {
	if block.Content == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "content",
			Message: "Comentario deve ter conteudo",
		})
	} else {
		validateNoDelimiters(block.Content, "content", blockID, errs)
	}
}

func validateSectionBlock(block *TemplateBlock, blockID string, depth int, errs *[]ValidationError) {
	if len(block.Children) == 0 {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "children",
			Message: "Secao deve ter blocos filhos",
		})
	} else {
		validateBlocksRecursive(block.Children, blockID, depth+1, errs)
	}
}

func validateWithBlock(block *TemplateBlock, blockID string, depth int, errs *[]ValidationError) {
	if block.Variable == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "variable",
			Message: "With deve ter uma variavel",
		})
	} else {
		validateIdentifier(block.Variable, "variable", blockID, errs)
	}

	if block.Assignment == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "assignment",
			Message: "With deve ter uma atribuicao",
		})
	} else {
		validateNoDelimiters(block.Assignment, "assignment", blockID, errs)
	}

	if len(block.Children) == 0 {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "children",
			Message: "With deve ter blocos filhos",
		})
	} else {
		validateBlocksRecursive(block.Children, blockID, depth+1, errs)
	}
}

func validateExpressionBlock(block *TemplateBlock, blockID string, errs *[]ValidationError) {
	if block.Expression == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "expression",
			Message: "Expressao deve ter uma expressao",
		})
	} else {
		validateNoDelimiters(block.Expression, "expression", blockID, errs)
	}
}

// validFilterNames is the set of recognized Pongo2 filter names.
var validFilterNames = map[string]bool{
	"percent_of":  true,
	"slice_str":   true,
	"strip_zeros": true,
	"replace":     true,
	"where":       true,
	"sum":         true,
	"count":       true,
	"floatformat": true,
	"length":      true,
	"date":        true,
}

// validCustomTags is the set of recognized custom Pongo2 tags.
var validCustomTags = map[string]bool{
	"sum_by":             true,
	"count_by":           true,
	"avg_by":             true,
	"min_by":             true,
	"max_by":             true,
	"calc":               true,
	"last_item_by_group": true,
}

func validateCustomTagBlock(block *TemplateBlock, blockID string, errs *[]ValidationError) {
	if block.TagName == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "tagName",
			Message: "Tag customizada deve ter um nome (tagName)",
		})
	} else if !validCustomTags[block.TagName] {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "tagName",
			Message: fmt.Sprintf("Tag customizada desconhecida: %s", block.TagName),
		})
	}

	if block.TagArgs == "" {
		*errs = append(*errs, ValidationError{
			BlockID: blockID,
			Field:   "tagArgs",
			Message: "Tag customizada deve ter argumentos (tagArgs)", //nolint:misspell // Portuguese
		})
	} else {
		validateNoDelimiters(block.TagArgs, "tagArgs", blockID, errs)
	}
}
