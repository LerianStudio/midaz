// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package middleware

import (
	"os"
	"time"

	"github.com/gofiber/fiber/v2"

	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// FaultInjectionConfig holds configuration for the fault injection middleware.
type FaultInjectionConfig struct {
	// Enabled controls whether fault injection is active.
	// Should only be true in integration test environments.
	Enabled bool

	// TimeoutDuration is how long to wait before returning the timeout error.
	// Default is 100ms (enough to trigger context deadline).
	TimeoutDuration time.Duration
}

// DefaultFaultInjectionConfig returns the default configuration.
func DefaultFaultInjectionConfig() FaultInjectionConfig {
	return FaultInjectionConfig{
		Enabled:         os.Getenv("FAULT_INJECTION_ENABLED") == "true",
		TimeoutDuration: 100 * time.Millisecond,
	}
}

// FaultInjection returns a middleware that can simulate infrastructure failures.
// This middleware is ONLY for integration testing and should NEVER be enabled in production.
//
// Usage in tests:
//
//	req.Header.Set("X-Test-Fault-Injection", "timeout")     // gateway timeout (504)
//	req.Header.Set("X-Test-Fault-Injection", "unavailable") // service unavailable (503)
func FaultInjection(config ...FaultInjectionConfig) fiber.Handler {
	cfg := DefaultFaultInjectionConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(c *fiber.Ctx) error {
		// Skip if not enabled (production safety)
		if !cfg.Enabled {
			return c.Next()
		}

		// Check for fault injection header
		faultType := c.Get(trcConstant.FaultInjectionHeader)
		if faultType == "" {
			return c.Next()
		}

		switch faultType {
		case trcConstant.FaultTimeout:
			// Simulate processing that exceeds timeout
			time.Sleep(cfg.TimeoutDuration)

			// Return the appropriate timeout code based on endpoint.
			// GET /v1/validations (list) maps to the list-validations timeout,
			// POST /v1/validations to the validation timeout.
			if c.Method() == "GET" && c.Path() == "/v1/validations" {
				return pkgHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrListValidationsTimeout, constant.EntityTransactionValidation))
			}

			return pkgHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrValidationTimeout, constant.EntityValidationRequest))

		case trcConstant.FaultUnavailable:
			return pkgHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrContextCancelled, constant.EntityRule))

		default:
			// Unknown fault type, continue normally
			return c.Next()
		}
	}
}
