// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"sync"

	clog "github.com/LerianStudio/lib-observability/log"
)

// CleanupManager provides a typed, thread-safe LIFO cleanup stack for graceful shutdown.
// Resources are registered as they are initialized, and executed in reverse order on cleanup.
type CleanupManager struct {
	mu     sync.Mutex
	stack  []cleanupEntry
	logger clog.Logger
}

type cleanupEntry struct {
	name string
	fn   func()
}

// NewCleanupManager creates a new CleanupManager.
func NewCleanupManager(logger clog.Logger) *CleanupManager {
	return &CleanupManager{logger: logger}
}

// Register adds a cleanup function to the stack with a descriptive name.
func (cm *CleanupManager) Register(name string, fn func()) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.stack = append(cm.stack, cleanupEntry{name: name, fn: fn})
}

// ExecuteAll runs all registered cleanup functions in reverse order (LIFO).
// Safe to call multiple times — each call drains the stack.
func (cm *CleanupManager) ExecuteAll() {
	cm.mu.Lock()
	stack := cm.stack
	cm.stack = nil
	cm.mu.Unlock()

	for i := len(stack) - 1; i >= 0; i-- {
		entry := stack[i]
		cm.logger.Log(context.Background(), clog.LevelInfo, "Cleanup: "+entry.name)
		entry.fn()
	}
}

// Len returns the number of registered cleanup functions.
func (cm *CleanupManager) Len() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	return len(cm.stack)
}
