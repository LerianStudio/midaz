// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// allAccountingEntryKeys reads the JSON key of every per-action entry that
// AccountingEntries declares, so a new action cannot be missed by the merges.
func allAccountingEntryKeys(t *testing.T) []string {
	t.Helper()

	entryType := reflect.TypeOf(&mmodel.AccountingEntry{})
	entriesType := reflect.TypeOf(mmodel.AccountingEntries{})

	keys := make([]string, 0, entriesType.NumField())
	for i := range entriesType.NumField() {
		field := entriesType.Field(i)
		if field.Type != entryType {
			continue
		}

		keys = append(keys, strings.Split(field.Tag.Get("json"), ",")[0])
	}

	require.NotEmpty(t, keys)

	return keys
}

// entriesWithDistinctRubrics fills every action with its own rubric code, keyed
// by the action's JSON key, and returns the JSON form alongside.
func entriesWithDistinctRubrics(t *testing.T, keys []string, prefix string) *mmodel.AccountingEntries {
	t.Helper()

	raw := make(map[string]any, len(keys))
	for _, key := range keys {
		raw[key] = map[string]any{
			"debit":  map[string]any{"code": prefix + "-" + key + "-debit", "description": key},
			"credit": map[string]any{"code": prefix + "-" + key + "-credit", "description": key},
		}
	}

	body, err := json.Marshal(raw)
	require.NoError(t, err)

	var entries mmodel.AccountingEntries
	require.NoError(t, json.Unmarshal(body, &entries))

	return &entries
}

func entriesByKey(t *testing.T, entries *mmodel.AccountingEntries) map[string]json.RawMessage {
	t.Helper()

	body, err := json.Marshal(entries)
	require.NoError(t, err)

	var byKey map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &byKey))

	return byKey
}

func TestMergeAccountingEntries_TreatsEveryActionAlike(t *testing.T) {
	t.Parallel()

	keys := allAccountingEntryKeys(t)

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			existing := entriesWithDistinctRubrics(t, keys, "stored")
			incoming := entriesWithDistinctRubrics(t, []string{key}, "new")
			stored := entriesByKey(t, existing)
			updated := entriesByKey(t, incoming)

			replaced := entriesByKey(t, mergeAccountingEntries(existing, incoming, json.RawMessage(`{"`+key+`":{}}`)))
			removed := entriesByKey(t, mergeAccountingEntries(existing, &mmodel.AccountingEntries{}, json.RawMessage(`{"`+key+`":null}`)))
			simple := entriesByKey(t, mergeAccountingEntriesSimple(existing, incoming))

			for _, other := range keys {
				if other == key {
					assert.JSONEq(t, string(updated[key]), string(replaced[key]), "merge patch replaces %s", key)
					assert.NotContains(t, removed, key, "explicit null removes %s", key)
					assert.JSONEq(t, string(updated[key]), string(simple[key]), "simple merge replaces %s", key)

					continue
				}

				assert.JSONEq(t, string(stored[other]), string(replaced[other]), "merge patch of %s keeps %s", key, other)
				assert.JSONEq(t, string(stored[other]), string(removed[other]), "null on %s keeps %s", key, other)
				assert.JSONEq(t, string(stored[other]), string(simple[other]), "simple merge of %s keeps %s", key, other)
			}
		})
	}
}

func TestMergeAccountingEntries_RemovingEveryActionLeavesNoEntries(t *testing.T) {
	t.Parallel()

	keys := allAccountingEntryKeys(t)

	nulls := make(map[string]any, len(keys))
	for _, key := range keys {
		nulls[key] = nil
	}

	body, err := json.Marshal(nulls)
	require.NoError(t, err)

	assert.Nil(t, mergeAccountingEntries(entriesWithDistinctRubrics(t, keys, "stored"), &mmodel.AccountingEntries{}, body))
}
