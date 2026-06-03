// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/LerianStudio/lib-observability/log"
)

// SafeDataSources provides thread-safe access to a map of DataSource values.
// It wraps a plain map[string]DataSource with a sync.RWMutex to prevent
// concurrent map read/write panics when multiple goroutines (health checker,
// request handlers, workers) access the datasource registry simultaneously.
type SafeDataSources struct {
	mu sync.RWMutex
	ds map[string]DataSource
}

// NewSafeDataSources creates a new SafeDataSources initialized with a shallow
// copy of the provided map. If initial is nil, an empty map is used.
func NewSafeDataSources(initial map[string]DataSource) *SafeDataSources {
	copied := make(map[string]DataSource, len(initial))
	for k, v := range initial {
		copied[k] = v
	}

	return &SafeDataSources{
		ds: copied,
	}
}

// Get retrieves a DataSource by name. Returns the DataSource and true if found,
// or a zero-value DataSource and false if not present. Safe to call on nil receiver.
func (s *SafeDataSources) Get(name string) (DataSource, bool) {
	if s == nil {
		return DataSource{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ds, ok := s.ds[name]

	return ds, ok
}

// Set inserts or updates a DataSource entry by name. Safe to call on nil receiver (no-op).
func (s *SafeDataSources) Set(name string, ds DataSource) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ds[name] = ds
}

// GetAll returns a shallow copy of the internal map. Modifications to the
// returned map do not affect the SafeDataSources internal state.
// Safe to call on nil receiver (returns empty map).
func (s *SafeDataSources) GetAll() map[string]DataSource {
	if s == nil {
		return make(map[string]DataSource)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	copied := make(map[string]DataSource, len(s.ds))
	for k, v := range s.ds {
		copied[k] = v
	}

	return copied
}

// Len returns the number of datasources currently stored.
// Safe to call on nil receiver (returns 0).
func (s *SafeDataSources) Len() int {
	if s == nil {
		return 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.ds)
}

// ConnectDataSource establishes a connection to a named data source, delegating
// to the package-level ConnectToDataSource function while holding the write lock.
// This ensures thread-safe mutation of the datasource entry after connection.
//
// The name parameter is defensively copied (strings.Clone) because callers may pass
// strings backed by fasthttp's reusable request buffers (via Fiber's c.Params()).
// Without the copy, the map key can be silently corrupted when the buffer is reused
// by a subsequent HTTP request.
func (s *SafeDataSources) ConnectDataSource(ctx context.Context, name string, ds *DataSource, logger log.Logger) error {
	if s == nil {
		return fmt.Errorf("cannot connect datasource %s: SafeDataSources is nil", name)
	}

	// Clone the name to detach it from any fasthttp buffer that may be reused.
	safeName := strings.Clone(name)

	s.mu.Lock()
	defer s.mu.Unlock()

	return ConnectToDataSource(ctx, safeName, ds, logger, s.ds)
}
