// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package template_builder

import (
	"errors"
	"fmt"
	"strings"
)

const (
	indentSize  = 4
	maxGenDepth = 50
)

// escapeQuoted escapes backslashes and double quotes so the value can be
// safely embedded inside a quoted template literal (e.g. |date:"value").
func escapeQuoted(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)

	return s
}

// GenerateCode converts a slice of TemplateBlock into Pongo2 template code.
// The format parameter controls optional wrapping (html, xml, txt, or empty for none).
func GenerateCode(blocks []TemplateBlock, format string) (string, error) {
	if len(blocks) == 0 {
		return "", errors.New("blocks must not be empty")
	}

	var sb strings.Builder

	if err := generateBlocks(&sb, blocks, 0, 0); err != nil {
		return "", err
	}

	body := sb.String()

	return wrapWithFormat(body, format), nil
}

// generateBlocks recursively generates code for a slice of blocks at the given indentation level.
func generateBlocks(sb *strings.Builder, blocks []TemplateBlock, indent, depth int) error {
	if depth > maxGenDepth {
		return fmt.Errorf("maximum nesting depth exceeded (%d levels)", maxGenDepth)
	}

	for _, block := range blocks {
		if err := generateBlock(sb, block, indent, depth); err != nil {
			return err
		}
	}

	return nil
}

// generateBlock generates code for a single block.
//
//nolint:cyclop // block type dispatch requires multiple branches
func generateBlock(sb *strings.Builder, block TemplateBlock, indent, depth int) error {
	switch block.Type {
	case "text":
		writeIndented(sb, block.Content, indent, block.Inline)
	case "variable":
		writeIndented(sb, buildVariableExpression(block), indent, block.Inline)
	case "loop":
		return generateLoopBlock(sb, block, indent, depth)
	case "conditional":
		return generateConditionalBlock(sb, block, indent, depth)
	case "aggregation":
		expr := fmt.Sprintf("{{ %s|%s:\"%s\" }}", block.Collection, block.Function, block.Field)
		writeIndented(sb, expr, indent, block.Inline)
	case "calculation":
		expr := fmt.Sprintf("{{ %s|%s }}", block.Variable, block.Function)
		writeIndented(sb, expr, indent, block.Inline)
	case "date_time":
		var expr string
		if block.Variable == "" {
			// Now mode: generate custom tag for current date
			expr = tagWrap(block.TrimWhitespace, fmt.Sprintf("date_time \"%s\"", escapeQuoted(block.Format)))
		} else {
			// Field mode: format an existing variable with the date filter
			expr = fmt.Sprintf("{{ %s|date:\"%s\" }}", block.Variable, escapeQuoted(block.Format))
		}

		writeIndented(sb, expr, indent, block.Inline)
	case "counter":
		return generateCounterBlock(sb, block, indent)
	case "comment":
		// Sanitize comment-closing sequence to prevent injection breakout.
		sanitized := strings.ReplaceAll(block.Content, "#}", "# }")
		expr := fmt.Sprintf("{# %s #}", sanitized)
		writeIndented(sb, expr, indent, block.Inline)
	case "section":
		return generateBlocks(sb, block.Children, indent, depth+1)
	case "with":
		return generateWithBlock(sb, block, indent, depth)
	case "expression":
		writeIndented(sb, fmt.Sprintf("{{ %s }}", block.Expression), indent, block.Inline)
	case "custom_tag":
		if !validCustomTags[block.TagName] {
			return fmt.Errorf("unknown custom tag: %s", block.TagName)
		}

		expr := tagWrap(block.TrimWhitespace, fmt.Sprintf("%s %s", block.TagName, block.TagArgs))
		writeIndented(sb, expr, indent, block.Inline)
	default:
		return fmt.Errorf("unknown block type: %s", block.Type)
	}

	return nil
}

// buildVariableExpression builds a {{ variable|filter1:"arg"|filter2 }} expression.
func buildVariableExpression(block TemplateBlock) string {
	var sb strings.Builder

	sb.WriteString("{{ ")
	sb.WriteString(block.Variable)

	for _, f := range block.Filters {
		sb.WriteString("|")
		sb.WriteString(f.Name)

		if f.Args != "" {
			sb.WriteString(":\"")
			sb.WriteString(escapeQuoted(f.Args))
			sb.WriteString("\"")
		}
	}

	sb.WriteString(" }}")

	return sb.String()
}

// generateLoopBlock generates a {% for ... %} ... {% endfor %} block.
func generateLoopBlock(sb *strings.Builder, block TemplateBlock, indent, depth int) error {
	openTag := tagWrap(block.TrimWhitespace, fmt.Sprintf("for %s in %s", block.Iterator, block.Collection))
	writeIndented(sb, openTag, indent, false)

	childIndent := indent + indentSize
	if block.TrimWhitespace {
		childIndent = 0
	}

	if err := generateBlocks(sb, block.Children, childIndent, depth+1); err != nil {
		return err
	}

	writeIndented(sb, tagWrap(block.TrimWhitespace, "endfor"), indent, false)

	return nil
}

// generateConditionalBlock generates a {% if ... %} ... {% endif %} block.
func generateConditionalBlock(sb *strings.Builder, block TemplateBlock, indent, depth int) error {
	openTag := tagWrap(block.TrimWhitespace, fmt.Sprintf("if %s", block.Condition))

	if block.Inline {
		return generateInlineConditional(sb, block, openTag, depth)
	}

	writeIndented(sb, openTag, indent, false)

	childIndent := indent + indentSize
	if block.TrimWhitespace {
		childIndent = 0
	}

	if err := generateBlocks(sb, block.Children, childIndent, depth+1); err != nil {
		return err
	}

	for _, elif := range block.ElifBranches {
		writeIndented(sb, tagWrap(block.TrimWhitespace, fmt.Sprintf("elif %s", elif.Condition)), indent, false)

		if err := generateBlocks(sb, elif.Children, childIndent, depth+1); err != nil {
			return err
		}
	}

	if len(block.ElseChildren) > 0 {
		writeIndented(sb, tagWrap(block.TrimWhitespace, "else"), indent, false)

		if err := generateBlocks(sb, block.ElseChildren, childIndent, depth+1); err != nil {
			return err
		}
	}

	writeIndented(sb, tagWrap(block.TrimWhitespace, "endif"), indent, false)

	return nil
}

// generateInlineConditional generates an inline conditional: {% if %}...{% elif %}...{% else %}...{% endif %} on a single line.
func generateInlineConditional(sb *strings.Builder, block TemplateBlock, openTag string, depth int) error {
	sb.WriteString(openTag)

	if err := generateBlocks(sb, block.Children, 0, depth+1); err != nil {
		return err
	}

	for _, elif := range block.ElifBranches {
		sb.WriteString(tagWrap(block.TrimWhitespace, fmt.Sprintf("elif %s", elif.Condition)))

		if err := generateBlocks(sb, elif.Children, 0, depth+1); err != nil {
			return err
		}
	}

	if len(block.ElseChildren) > 0 {
		sb.WriteString(tagWrap(block.TrimWhitespace, "else"))

		if err := generateBlocks(sb, block.ElseChildren, 0, depth+1); err != nil {
			return err
		}
	}

	sb.WriteString(tagWrap(block.TrimWhitespace, "endif"))

	return nil
}

// generateCounterBlock generates {% counter "name" %} or {% counter_show "n1" "n2" %}.
func generateCounterBlock(sb *strings.Builder, block TemplateBlock, indent int) error {
	if block.Properties == nil {
		return errors.New("counter block requires properties")
	}

	mode, _ := block.Properties["counterMode"].(string)
	namesRaw, _ := block.Properties["counterNames"].([]any)

	names := make([]string, 0, len(namesRaw))
	for _, n := range namesRaw {
		if s, ok := n.(string); ok {
			names = append(names, s)
		}
	}

	if len(names) == 0 {
		return errors.New("counter block requires at least one name in counterNames")
	}

	var expr string

	switch mode {
	case "increment":
		expr = tagWrap(block.TrimWhitespace, fmt.Sprintf("counter \"%s\"", names[0]))
	case "show":
		quoted := make([]string, len(names))
		for i, n := range names {
			quoted[i] = fmt.Sprintf("\"%s\"", n)
		}

		expr = tagWrap(block.TrimWhitespace, fmt.Sprintf("counter_show %s", strings.Join(quoted, " ")))
	default:
		return fmt.Errorf("unknown counter mode: %s", mode)
	}

	writeIndented(sb, expr, indent, block.Inline)

	return nil
}

// generateWithBlock generates a {% with var = expr %} ... {% endwith %} block.
func generateWithBlock(sb *strings.Builder, block TemplateBlock, indent, depth int) error {
	openTag := tagWrap(block.TrimWhitespace, fmt.Sprintf("with %s = %s", block.Variable, block.Assignment))
	writeIndented(sb, openTag, indent, false)

	childIndent := indent + indentSize
	if block.TrimWhitespace {
		childIndent = 0
	}

	if err := generateBlocks(sb, block.Children, childIndent, depth+1); err != nil {
		return err
	}

	writeIndented(sb, tagWrap(block.TrimWhitespace, "endwith"), indent, false)

	return nil
}

// tagWrap wraps content in {% %} or {%- -%} depending on the trim flag.
func tagWrap(trim bool, content string) string {
	if trim {
		return "{%- " + content + " -%}"
	}

	return "{% " + content + " %}"
}

// writeIndented writes content with optional indentation and newline.
func writeIndented(sb *strings.Builder, content string, indent int, inline bool) {
	if !inline && indent > 0 {
		sb.WriteString(strings.Repeat(" ", indent))
	}

	sb.WriteString(content)

	if !inline {
		sb.WriteString("\n")
	}
}

// wrapWithFormat wraps the generated body with format-specific headers/footers.
func wrapWithFormat(body string, format string) string {
	switch strings.ToLower(format) {
	case "html":
		return "<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"UTF-8\">\n</head>\n<body>\n" + body + "</body>\n</html>\n"
	case "xml":
		return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" + body
	default:
		return body
	}
}
