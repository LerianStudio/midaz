// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build property

package template_builder

import (
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

// --- GenerateCode Properties ---

func TestProperty_GenerateCode_NonEmptyOutput(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		if content == "" {
			content = "fallback"
		}

		blocks := []TemplateBlock{{Type: "text", Content: content}}

		code, err := GenerateCode(blocks, "")

		return err == nil && len(code) > 0
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_GenerateCode_TextBlockPreservesContent(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		if content == "" {
			content = "some text"
		}

		blocks := []TemplateBlock{{Type: "text", Content: content}}

		code, err := GenerateCode(blocks, "")

		return err == nil && strings.Contains(code, content)
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_GenerateCode_VariableBlockContainsBraces(t *testing.T) {
	t.Parallel()

	f := func(variable string) bool {
		if variable == "" {
			variable = "item.name"
		}

		blocks := []TemplateBlock{{Type: "variable", Variable: variable}}

		code, err := GenerateCode(blocks, "")

		return err == nil && strings.Contains(code, "{{ ") && strings.Contains(code, " }}")
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_GenerateCode_LoopBlockContainsForAndEndfor(t *testing.T) {
	t.Parallel()

	f := func(iterator string, collection string) bool {
		if iterator == "" {
			iterator = "item"
		}

		if collection == "" {
			collection = "items"
		}

		blocks := []TemplateBlock{{
			Type:       "loop",
			Iterator:   iterator,
			Collection: collection,
			Children:   []TemplateBlock{{Type: "text", Content: "body"}},
		}}

		code, err := GenerateCode(blocks, "")

		return err == nil && strings.Contains(code, "{% for") && strings.Contains(code, "{% endfor %}")
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_GenerateCode_ConditionalBlockContainsIfAndEndif(t *testing.T) {
	t.Parallel()

	f := func(condition string) bool {
		if condition == "" {
			condition = "x > 0"
		}

		blocks := []TemplateBlock{{
			Type:      "conditional",
			Condition: condition,
			Children:  []TemplateBlock{{Type: "text", Content: "yes"}},
		}}

		code, err := GenerateCode(blocks, "")

		return err == nil && strings.Contains(code, "{% if") && strings.Contains(code, "{% endif %}")
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_GenerateCode_CommentBlockContainsCommentSyntax(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		if content == "" {
			content = "a comment"
		}

		blocks := []TemplateBlock{{Type: "comment", Content: content}}

		code, err := GenerateCode(blocks, "")

		return err == nil && strings.Contains(code, "{#") && strings.Contains(code, "#}")
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_GenerateCode_CounterIncrementContainsCounterTag(t *testing.T) {
	t.Parallel()

	f := func(name string) bool {
		if name == "" {
			name = "c1"
		}

		blocks := []TemplateBlock{{
			Type: "counter",
			Properties: map[string]interface{}{
				"counterMode":  "increment",
				"counterNames": []interface{}{name},
			},
		}}

		code, err := GenerateCode(blocks, "")

		return err == nil && strings.Contains(code, "{% counter")
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_GenerateCode_CounterShowContainsCounterShowTag(t *testing.T) {
	t.Parallel()

	f := func(name string) bool {
		if name == "" {
			name = "c1"
		}

		blocks := []TemplateBlock{{
			Type: "counter",
			Properties: map[string]interface{}{
				"counterMode":  "show",
				"counterNames": []interface{}{name},
			},
		}}

		code, err := GenerateCode(blocks, "")

		return err == nil && strings.Contains(code, "{% counter_show")
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_GenerateCode_InlineBlockNoTrailingNewline(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		if content == "" {
			content = "inline text"
		}

		blocks := []TemplateBlock{{Type: "text", Content: content, Inline: true}}

		code, err := GenerateCode(blocks, "")

		return err == nil && !strings.HasSuffix(code, "\n")
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_GenerateCode_HTMLFormatContainsDoctype(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		if content == "" {
			content = "hello"
		}

		blocks := []TemplateBlock{{Type: "text", Content: content}}

		code, err := GenerateCode(blocks, "html")

		return err == nil && strings.Contains(code, "<!DOCTYPE html>")
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_GenerateCode_XMLFormatContainsDeclaration(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		if content == "" {
			content = "hello"
		}

		blocks := []TemplateBlock{{Type: "text", Content: content}}

		code, err := GenerateCode(blocks, "xml")

		return err == nil && strings.Contains(code, "<?xml")
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

// --- ExtractMappedFields Properties ---

func TestProperty_ExtractMappedFields_NonNilResult(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		blocks := []TemplateBlock{{Type: "text", Content: content}}

		result := ExtractMappedFields(blocks)

		return result != nil
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_ExtractMappedFields_VariableBlockPopulatesFields(t *testing.T) {
	t.Parallel()

	f := func(table string, field string) bool {
		if table == "" || field == "" || strings.Contains(table, ".") || strings.Contains(field, ".") {
			table = "users"
			field = "name"
		}

		variable := table + "." + field

		blocks := []TemplateBlock{{Type: "variable", Variable: variable}}

		result := ExtractMappedFields(blocks)

		ds, ok := result["default"]
		if !ok {
			return false
		}

		fields, ok := ds[table]
		if !ok {
			return false
		}

		for _, f := range fields {
			if f == field {
				return true
			}
		}

		return false
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_ExtractMappedFields_LoopBlockPopulatesCollection(t *testing.T) {
	t.Parallel()

	f := func(collection string) bool {
		if collection == "" {
			collection = "orders"
		}

		blocks := []TemplateBlock{{
			Type:       "loop",
			Iterator:   "item",
			Collection: collection,
			Children:   []TemplateBlock{{Type: "text", Content: "body"}},
		}}

		result := ExtractMappedFields(blocks)

		ds, ok := result["default"]
		if !ok {
			return false
		}

		_, ok = ds[collection]

		return ok
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_ExtractMappedFields_Idempotent(t *testing.T) {
	t.Parallel()

	f := func(table string, field string) bool {
		if table == "" || field == "" || strings.Contains(table, ".") || strings.Contains(field, ".") {
			table = "accounts"
			field = "balance"
		}

		blocks := []TemplateBlock{
			{Type: "variable", Variable: table + "." + field},
			{Type: "loop", Iterator: "x", Collection: "items", Children: []TemplateBlock{{Type: "text", Content: "row"}}},
		}

		result1 := ExtractMappedFields(blocks)
		result2 := ExtractMappedFields(blocks)

		return reflect.DeepEqual(result1, result2)
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}
