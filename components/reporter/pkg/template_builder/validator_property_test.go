// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build property

package template_builder

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

// --- ValidateBlocks Properties ---

func TestProperty_Validate_ValidBlocksReturnValid(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		if content == "" {
			content = "fallback"
		}

		blocks := []TemplateBlock{{Type: "text", Content: content}}
		result := ValidateBlocks(blocks)

		return result.Valid && len(result.Errors) == 0
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_Validate_EmptyTypeAlwaysInvalid(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		blocks := []TemplateBlock{{Type: "", Content: content}}
		result := ValidateBlocks(blocks)

		if result.Valid {
			return false
		}

		for _, e := range result.Errors {
			if strings.Contains(strings.ToLower(e.Message), "tipo de bloco") ||
				strings.Contains(strings.ToLower(e.Field), "type") {
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

func TestProperty_Validate_TextWithoutContentInvalid(t *testing.T) {
	t.Parallel()

	f := func(blockID string) bool {
		blocks := []TemplateBlock{{BlockID: blockID, Type: "text", Content: ""}}
		result := ValidateBlocks(blocks)

		return !result.Valid && len(result.Errors) > 0
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_Validate_LoopWithoutChildrenInvalid(t *testing.T) {
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
			Children:   nil,
		}}
		result := ValidateBlocks(blocks)

		if result.Valid {
			return false
		}

		for _, e := range result.Errors {
			if e.Field == "children" {
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

func TestProperty_Validate_CounterInvalidModeInvalid(t *testing.T) {
	t.Parallel()

	f := func(mode string) bool {
		if mode == "increment" || mode == "show" || mode == "" {
			mode = "invalid_mode"
		}

		blocks := []TemplateBlock{{
			Type: "counter",
			Properties: map[string]interface{}{
				"counterMode":  mode,
				"counterNames": []interface{}{"c1"},
			},
		}}
		result := ValidateBlocks(blocks)

		if result.Valid {
			return false
		}

		for _, e := range result.Errors {
			if e.Field == "counterMode" && strings.Contains(e.Message, "increment") {
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

func TestProperty_Validate_CounterEmptyNamesInvalid(t *testing.T) {
	t.Parallel()

	f := func(mode bool) bool {
		counterMode := "increment"
		if mode {
			counterMode = "show"
		}

		blocks := []TemplateBlock{{
			Type: "counter",
			Properties: map[string]interface{}{
				"counterMode":  counterMode,
				"counterNames": []interface{}{},
			},
		}}
		result := ValidateBlocks(blocks)

		if result.Valid {
			return false
		}

		for _, e := range result.Errors {
			if e.Field == "counterNames" {
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

func TestProperty_Validate_SectionWithoutChildrenInvalid(t *testing.T) {
	t.Parallel()

	f := func(blockID string) bool {
		blocks := []TemplateBlock{{
			BlockID:  blockID,
			Type:     "section",
			Children: nil,
		}}
		result := ValidateBlocks(blocks)

		if result.Valid {
			return false
		}

		for _, e := range result.Errors {
			if e.Field == "children" {
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

func TestProperty_Validate_ValidResponseAlwaysHasEmptyErrors(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		if content == "" {
			content = "some content"
		}

		blocks := []TemplateBlock{{Type: "text", Content: content}}
		result := ValidateBlocks(blocks)

		if result.Valid {
			return len(result.Errors) == 0
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_Validate_InvalidResponseAlwaysHasErrors(t *testing.T) {
	t.Parallel()

	f := func(blockType string) bool {
		// Use empty content to force invalid for known types, or unknown type
		blocks := []TemplateBlock{{Type: blockType, Content: ""}}
		result := ValidateBlocks(blocks)

		if !result.Valid {
			return len(result.Errors) > 0
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_Validate_BlockIDPreserved(t *testing.T) {
	t.Parallel()

	f := func(blockID string) bool {
		if blockID == "" {
			blockID = "my-block-id"
		}

		// Use empty content to guarantee a validation error
		blocks := []TemplateBlock{{BlockID: blockID, Type: "text", Content: ""}}
		result := ValidateBlocks(blocks)

		if result.Valid || len(result.Errors) == 0 {
			return false
		}

		return result.Errors[0].BlockID == blockID
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_Validate_BlockIDGenerated(t *testing.T) {
	t.Parallel()

	f := func(idx uint8) bool {
		// Build a slice where the target block is at position idx (mod 5 to keep it small)
		position := int(idx % 5)
		blocks := make([]TemplateBlock, position+1)

		for i := 0; i < position; i++ {
			blocks[i] = TemplateBlock{Type: "text", Content: "valid"}
		}

		// The block at position has no BlockID and will fail validation
		blocks[position] = TemplateBlock{Type: "text", Content: ""}

		result := ValidateBlocks(blocks)
		if result.Valid {
			return false
		}

		expected := fmt.Sprintf("block-%d", position)

		for _, e := range result.Errors {
			if e.BlockID == expected {
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

func TestProperty_Validate_PureFunction(t *testing.T) {
	t.Parallel()

	f := func(content string) bool {
		blocks := []TemplateBlock{
			{Type: "text", Content: content},
			{Type: "variable", Variable: content},
		}

		result1 := ValidateBlocks(blocks)
		result2 := ValidateBlocks(blocks)

		return reflect.DeepEqual(result1, result2)
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("property violated: %v", err)
	}
}

func TestProperty_Validate_RecursiveValidation(t *testing.T) {
	t.Parallel()

	f := func(iterator string, collection string) bool {
		if iterator == "" {
			iterator = "item"
		}

		if collection == "" {
			collection = "items"
		}

		// Valid loop with an invalid child (text block with empty content)
		blocks := []TemplateBlock{{
			Type:       "loop",
			Iterator:   iterator,
			Collection: collection,
			Children: []TemplateBlock{
				{Type: "text", Content: ""},
			},
		}}

		result := ValidateBlocks(blocks)

		if result.Valid {
			return false
		}

		for _, e := range result.Errors {
			if e.Field == "content" {
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
