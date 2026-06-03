// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build property

package pongo

import (
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/assert"
)

// TestProperty_BlockDefinition_NonEmptyType verifies the invariant that every
// block definition returned by GetBlockDefinitions has a non-empty Type field
// across all invocations.
func TestProperty_BlockDefinition_NonEmptyType(t *testing.T) {
	f := func() bool {
		blocks := GetBlockDefinitions()
		for _, b := range blocks {
			if b.Type == "" {
				return false
			}
		}

		return len(blocks) > 0
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: all block definitions must have non-empty Type: %v", err)
	}
}

// TestProperty_BlockDefinition_NonEmptyLabel verifies the invariant that every
// block definition returned by GetBlockDefinitions has a non-empty Label field.
func TestProperty_BlockDefinition_NonEmptyLabel(t *testing.T) {
	f := func() bool {
		blocks := GetBlockDefinitions()
		for _, b := range blocks {
			if b.Label == "" {
				return false
			}
		}

		return len(blocks) > 0
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: all block definitions must have non-empty Label: %v", err)
	}
}

// TestProperty_BlockDefinition_AtLeastOneProperty verifies that block definitions
// which declare properties have at least one property entry. For blocks without
// explicit properties (basic blocks), the invariant is that Properties is either
// nil/empty (no properties needed) or has at least one entry.
func TestProperty_BlockDefinition_CounterHasProperties(t *testing.T) {
	f := func() bool {
		blocks := GetBlockDefinitions()
		for _, b := range blocks {
			if b.Type == "counter" && len(b.Properties) == 0 {
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: counter block must have at least one property: %v", err)
	}
}

// TestProperty_FilterDefinition_NonEmptyName verifies that every filter definition
// returned by GetFilterDefinitions has a non-empty Name field.
func TestProperty_FilterDefinition_NonEmptyName(t *testing.T) {
	f := func() bool {
		filters := GetFilterDefinitions()
		for _, f := range filters {
			if f.Name == "" {
				return false
			}
		}

		return len(filters) > 0
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: all filter definitions must have non-empty Name: %v", err)
	}
}

// TestProperty_FilterDefinition_NonEmptyCategory verifies that every filter
// definition has a non-empty Description field (serving as its category context).
func TestProperty_FilterDefinition_NonEmptyDescription(t *testing.T) {
	f := func() bool {
		filters := GetFilterDefinitions()
		for _, f := range filters {
			if f.Description == "" {
				return false
			}
		}

		return len(filters) > 0
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: all filter definitions must have non-empty Description: %v", err)
	}
}

// TestProperty_BlockDefinition_PureFunction verifies that GetBlockDefinitions is
// a pure function: calling it multiple times always returns the same result.
func TestProperty_BlockDefinition_PureFunction(t *testing.T) {
	f := func() bool {
		a := GetBlockDefinitions()
		b := GetBlockDefinitions()

		if len(a) != len(b) {
			return false
		}

		for i := range a {
			if a[i].Type != b[i].Type ||
				a[i].Label != b[i].Label ||
				a[i].Category != b[i].Category ||
				a[i].AcceptsChildren != b[i].AcceptsChildren ||
				len(a[i].Properties) != len(b[i].Properties) {
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: GetBlockDefinitions must be a pure function: %v", err)
	}
}

// TestProperty_FilterDefinition_PureFunction verifies that GetFilterDefinitions is
// a pure function: calling it multiple times always returns the same result.
func TestProperty_FilterDefinition_PureFunction(t *testing.T) {
	f := func() bool {
		a := GetFilterDefinitions()
		b := GetFilterDefinitions()

		if len(a) != len(b) {
			return false
		}

		for i := range a {
			if a[i].Name != b[i].Name ||
				a[i].Description != b[i].Description ||
				a[i].Example != b[i].Example ||
				len(a[i].Args) != len(b[i].Args) {
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: GetFilterDefinitions must be a pure function: %v", err)
	}
}

// TestProperty_BlockDefinition_SectionAcceptsChildren verifies that the "section"
// block type always has AcceptsChildren set to true.
func TestProperty_BlockDefinition_SectionAcceptsChildren(t *testing.T) {
	f := func() bool {
		blocks := GetBlockDefinitions()
		for _, b := range blocks {
			if b.Type == "section" && !b.AcceptsChildren {
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: section block must have acceptsChildren=true: %v", err)
	}
}

// TestProperty_BlockDefinition_CounterHasCounterModeAndCounterNames verifies that
// the "counter" block type always has both counterMode and counterNames properties.
func TestProperty_BlockDefinition_CounterHasCounterModeAndCounterNames(t *testing.T) {
	f := func() bool {
		blocks := GetBlockDefinitions()
		for _, b := range blocks {
			if b.Type != "counter" {
				continue
			}

			hasCounterMode := false
			hasCounterNames := false

			for _, p := range b.Properties {
				if p.Name == "counterMode" {
					hasCounterMode = true
				}

				if p.Name == "counterNames" {
					hasCounterNames = true
				}
			}

			if !hasCounterMode || !hasCounterNames {
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: counter block must have counterMode and counterNames: %v", err)
	}
}

// TestProperty_BlockDefinition_UniqueTypes verifies that all block type names are
// unique across all definitions.
func TestProperty_BlockDefinition_UniqueTypes(t *testing.T) {
	f := func() bool {
		blocks := GetBlockDefinitions()
		seen := make(map[string]bool, len(blocks))

		for _, b := range blocks {
			if seen[b.Type] {
				return false
			}

			seen[b.Type] = true
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: block type names must be unique: %v", err)
	}
}

// TestProperty_FilterDefinition_UniqueNames verifies that all filter names are
// unique across all definitions.
func TestProperty_FilterDefinition_UniqueNames(t *testing.T) {
	f := func() bool {
		filters := GetFilterDefinitions()
		seen := make(map[string]bool, len(filters))

		for _, f := range filters {
			if seen[f.Name] {
				return false
			}

			seen[f.Name] = true
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: filter names must be unique: %v", err)
	}
}

// TestProperty_FilterDefinition_DIMPFiltersExist verifies that all required DIMP
// filters (replace, where, sum, count) are always present in filter definitions.
func TestProperty_FilterDefinition_DIMPFiltersExist(t *testing.T) {
	dimpFilters := []string{"replace", "where", "sum", "count"}

	f := func() bool {
		filters := GetFilterDefinitions()
		names := make(map[string]bool, len(filters))

		for _, f := range filters {
			names[f.Name] = true
		}

		for _, required := range dimpFilters {
			if !names[required] {
				return false
			}
		}

		return true
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: DIMP filters must exist: %v", err)
	}
}

// TestProperty_BlockDefinition_NonEmptyCategory verifies that every block definition
// has a non-empty Category field.
func TestProperty_BlockDefinition_NonEmptyCategory(t *testing.T) {
	f := func() bool {
		blocks := GetBlockDefinitions()
		for _, b := range blocks {
			if b.Category == "" {
				return false
			}
		}

		return len(blocks) > 0
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: all block definitions must have non-empty Category: %v", err)
	}
}

// TestProperty_BlockDefinition_Idempotent verifies that GetBlockDefinitions is
// idempotent: the result is structurally identical no matter how many times called.
// This uses assert for deep equality comparison.
func TestProperty_BlockDefinition_Idempotent(t *testing.T) {
	baseline := GetBlockDefinitions()

	f := func() bool {
		current := GetBlockDefinitions()
		return assert.ObjectsAreEqual(baseline, current)
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: GetBlockDefinitions must be idempotent: %v", err)
	}
}

// TestProperty_FilterDefinition_Idempotent verifies that GetFilterDefinitions is
// idempotent: the result is structurally identical no matter how many times called.
func TestProperty_FilterDefinition_Idempotent(t *testing.T) {
	baseline := GetFilterDefinitions()

	f := func() bool {
		current := GetFilterDefinitions()
		return assert.ObjectsAreEqual(baseline, current)
	}

	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("property violated: GetFilterDefinitions must be idempotent: %v", err)
	}
}
