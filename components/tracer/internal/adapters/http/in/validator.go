// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"sync"

	"github.com/go-playground/validator/v10"
)

// Singleton validator instance - thread-safe initialization.
// This is shared across all validation files in this package.
var (
	validate     *validator.Validate
	validateOnce sync.Once
	validateErr  error
)

// getValidator returns the singleton validator instance.
// Uses sync.Once for thread-safe lazy initialization.
// Registers all custom validations (rules and limits) on first call.
// Returns an error if validator initialization fails.
// Only assigns to global validate after all registrations succeed to prevent
// callers from using an incomplete validator if they ignore the error.
func getValidator() (*validator.Validate, error) {
	validateOnce.Do(func() {
		v := validator.New()

		if err := registerRuleValidations(v); err != nil {
			validateErr = fmt.Errorf("failed to register rule validations: %w", err)
			return
		}

		if err := registerLimitValidations(v); err != nil {
			validateErr = fmt.Errorf("failed to register limit validations: %w", err)
			return
		}

		if err := registerAuditEventValidations(v); err != nil {
			validateErr = fmt.Errorf("failed to register audit event validations: %w", err)
			return
		}

		// Only assign to global after all registrations succeed
		validate = v
	})

	return validate, validateErr
}
