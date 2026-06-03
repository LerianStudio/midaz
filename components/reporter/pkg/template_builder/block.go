// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template_builder

// TemplateBlock represents a single block in the template builder JSON structure.
type TemplateBlock struct {
	BlockID        string          `json:"blockId,omitempty"`
	Type           string          `json:"type"`
	Content        string          `json:"content,omitempty"`
	Variable       string          `json:"variable,omitempty"`
	Filters        []FilterChain   `json:"filters,omitempty"`
	Iterator       string          `json:"iterator,omitempty"`
	Collection     string          `json:"collection,omitempty"`
	Condition      string          `json:"condition,omitempty"`
	Function       string          `json:"function,omitempty"`
	Field          string          `json:"field,omitempty"`
	Format         string          `json:"format,omitempty"`
	Properties     map[string]any  `json:"properties,omitempty"`
	Children       []TemplateBlock `json:"children,omitempty"`
	Inline         bool            `json:"inline,omitempty"`
	TrimWhitespace bool            `json:"trimWhitespace,omitempty"`
	ElseChildren   []TemplateBlock `json:"elseChildren,omitempty"`
	ElifBranches   []ElifBranch    `json:"elifBranches,omitempty"`
	Assignment     string          `json:"assignment,omitempty"`
	Expression     string          `json:"expression,omitempty"`
	TagName        string          `json:"tagName,omitempty"`
	TagArgs        string          `json:"tagArgs,omitempty"`
}

// ElifBranch represents a single elif branch with its condition and children.
type ElifBranch struct {
	Condition string          `json:"condition"`
	Children  []TemplateBlock `json:"children"`
}

// FilterChain represents a single filter with optional arguments in the filter pipeline.
type FilterChain struct {
	Name string `json:"name"`
	Args string `json:"args,omitempty"`
}

// ValidationError represents a single validation error for a specific block.
type ValidationError struct {
	BlockID string `json:"blockId"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidateBlocksInput is the request payload for the validate endpoint.
type ValidateBlocksInput struct {
	Blocks []TemplateBlock `json:"blocks"`
}

// ValidateBlocksResponse is the response payload for the validate endpoint.
type ValidateBlocksResponse struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// GenerateCodeInput is the request payload for the generate-code endpoint.
type GenerateCodeInput struct {
	Blocks []TemplateBlock `json:"blocks"`
	Format string          `json:"format"`
}

// GenerateCodeResponse is the response payload for the generate-code endpoint.
type GenerateCodeResponse struct {
	Code         string                         `json:"code"`
	MappedFields map[string]map[string][]string `json:"mappedFields"`
}
