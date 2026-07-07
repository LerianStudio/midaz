// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

import "sync"

// indexState tracks whether indexes have been successfully created for a specific database/collection pair.
type indexState struct {
	mu   sync.Mutex
	done bool
}

// indexTracker manages per-database index creation state.
// In multi-tenant mode, each tenant database needs its own indexes.
// This tracker ensures indexes are created exactly once per database, with retry on failure.
type indexTracker struct {
	states sync.Map // key: "dbName:collection" -> *indexState
}

// ensureOnce executes fn exactly once per key, but only marks as done on success.
// If fn returns an error, subsequent calls will retry.
func (t *indexTracker) ensureOnce(key string, fn func() error) error {
	v, _ := t.states.LoadOrStore(key, &indexState{})
	state := v.(*indexState)

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.done {
		return nil
	}

	if err := fn(); err != nil {
		return err
	}

	state.done = true

	return nil
}

// reset clears the state for a specific key. Used in integration tests to ensure fresh state
// when each test runs with a new MongoDB container.
//
//nolint:unused // used in *_integration_test.go files (build tag: integration)
func (t *indexTracker) reset(key string) {
	t.states.Delete(key)
}

// globalIndexTracker is shared across all audit repository instances.
// This ensures indexes are created once per database even if multiple repository instances exist.
var globalIndexTracker = &indexTracker{}
