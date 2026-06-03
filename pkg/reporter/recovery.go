// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pkg

import (
	"context"
	"runtime/debug"

	"github.com/LerianStudio/lib-observability/log"
)

// Go starts a goroutine with panic recovery and stack trace logging.
// If the goroutine panics, it logs the panic value and stack trace
// instead of crashing the process.
func Go(logger log.Logger, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Log(context.Background(), log.LevelError, "Goroutine panic recovered", log.Any("panic", r), log.String("stack", string(debug.Stack())))
			}
		}()

		fn()
	}()
}

// GoWithCleanup starts a goroutine with panic recovery that also executes
// a cleanup function when a panic occurs. This is useful for goroutines
// that hold resources or need to trigger cancellation on failure.
func GoWithCleanup(logger log.Logger, fn func(), cleanup func(recovered any)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Log(context.Background(), log.LevelError, "Goroutine panic recovered", log.Any("panic", r), log.String("stack", string(debug.Stack())))

				if cleanup != nil {
					cleanup(r)
				}
			}
		}()

		fn()
	}()
}

// GoNamed starts a named goroutine with panic recovery and stack trace logging.
// The name is included in the log message for easier identification during debugging.
func GoNamed(logger log.Logger, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Log(context.Background(), log.LevelError, "Goroutine panic recovered", log.String("name", name), log.Any("panic", r), log.String("stack", string(debug.Stack())))
			}
		}()

		fn()
	}()
}
