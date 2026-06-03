// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pongo

import (
	"fmt"
	"sync"

	"github.com/flosch/pongo2/v6"
)

var registerOnce sync.Once

// RegisterAll registers all custom filters and tags for the Pongo2 template engine.
// It is safe to call multiple times; registration happens only once.
func RegisterAll() error {
	var registerErr error

	registerOnce.Do(func() {
		registerErr = doRegisterAll()
	})

	return registerErr
}

// doRegisterAll performs the actual registration of custom filters and tags.
func doRegisterAll() error {
	if err := pongo2.RegisterFilter("percent_of", percentOfFilter); err != nil {
		return fmt.Errorf("failed to register percent_of filter: %w", err)
	}

	if err := pongo2.RegisterFilter("slice_str", sliceFilter); err != nil {
		return fmt.Errorf("failed to register slice_str filter: %w", err)
	}

	if err := pongo2.RegisterFilter("strip_zeros", stripZerosFilter); err != nil {
		return fmt.Errorf("failed to register strip_zeros filter: %w", err)
	}

	if err := pongo2.RegisterFilter("replace", replaceFilter); err != nil {
		return fmt.Errorf("failed to register replace filter: %w", err)
	}

	if err := pongo2.RegisterFilter("where", whereFilter); err != nil {
		return fmt.Errorf("failed to register where filter: %w", err)
	}

	if err := pongo2.RegisterFilter("sum", sumFilter); err != nil {
		return fmt.Errorf("failed to register sum filter: %w", err)
	}

	if err := pongo2.RegisterFilter("count", countFilter); err != nil {
		return fmt.Errorf("failed to register count filter: %w", err)
	}

	if err := pongo2.RegisterFilter("add_day", addDayFilter); err != nil {
		return fmt.Errorf("failed to register add_day filter: %w", err)
	}

	if err := pongo2.RegisterFilter("add_signed", addSignedFilter); err != nil {
		return fmt.Errorf("failed to register add_signed filter: %w", err)
	}

	// Override the built-in |date filter to accept both time.Time and string inputs.
	// In direct mode, the PostgreSQL driver returns time.Time natively.
	// In fetcher mode, json.Unmarshal into map[string]any returns dates as strings.
	// This override transparently handles both without pipeline changes.
	if err := pongo2.ReplaceFilter("date", dateFilterWithStringSupport); err != nil {
		return fmt.Errorf("failed to replace date filter: %w", err)
	}

	tags := []struct {
		name string
		op   string
	}{
		{"sum_by", "sum"},
		{"count_by", "count"},
		{"avg_by", "avg"},
		{"min_by", "min"},
		{"max_by", "max"},
		{"date_time", "date"},
	}

	for _, tag := range tags {
		var err error

		if tag.op == "date" {
			err = pongo2.RegisterTag(tag.name, makeDateNowTag())
		} else {
			err = pongo2.RegisterTag(tag.name, makeAggregateTag(tag.op))
		}

		if err != nil {
			return fmt.Errorf("failed to register tag '%s': %w", tag.name, err)
		}
	}

	if err := pongo2.RegisterTag("calc", makeCalcTag); err != nil {
		return fmt.Errorf("failed to register calc tag: %w", err)
	}

	if err := pongo2.RegisterTag("last_item_by_group", makeLastItemByGroupTag()); err != nil {
		return fmt.Errorf("failed to register last_item_by_group tag: %w", err)
	}

	// Register counter tags for counting blocks during rendering
	if err := pongo2.RegisterTag("counter", makeCounterTag()); err != nil {
		return fmt.Errorf("failed to register counter tag: %w", err)
	}

	if err := pongo2.RegisterTag("counter_show", makeCounterShowTag()); err != nil {
		return fmt.Errorf("failed to register counter_show tag: %w", err)
	}

	return nil
}

// SafeFromString parses a template string using a fresh pongo2.TemplateSet,
// avoiding a data race on the global DefaultSet's unsynchronized
// firstTemplateCreated field.  Filters and tags are registered globally in
// pongo2 so they are available on every TemplateSet.
func SafeFromString(tpl string) (*pongo2.Template, error) {
	ts := newSafeTemplateSet("safe")
	return ts.FromString(tpl)
}

// bannedTags contains tags that are blocked to prevent Server-Side Template Injection.
// Covers file inclusion (include, ssi), template inheritance (extends, block), and imports (import, from).
var bannedTags = []string{"include", "extends", "import", "block", "ssi"}

// newSafeTemplateSet creates a pongo2.TemplateSet with dangerous tags banned.
// Panics if any tag cannot be banned, ensuring fail-closed security behavior.
func newSafeTemplateSet(name string) *pongo2.TemplateSet {
	ts := pongo2.NewSet(name, pongo2.DefaultLoader)

	for _, tag := range bannedTags {
		if err := ts.BanTag(tag); err != nil {
			panic(fmt.Sprintf("SECURITY: failed to ban tag %q: %v", tag, err))
		}
	}

	return ts
}
