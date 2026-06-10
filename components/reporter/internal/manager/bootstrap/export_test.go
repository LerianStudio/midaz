// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package bootstrap

import (
	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/gofiber/fiber/v2"
)

// initTenantMiddlewareForTest exercises initTenantMiddleware with the given
// enabled flag, envName, and URL, returning the fiber.Handler or nil.
// Uses log.NopLogger to avoid requiring a real logging backend in unit tests.
func initTenantMiddlewareForTest(enabled bool, envName, url string) (fiber.Handler, error) {
	cfg := &Config{
		MultiTenantEnabled: enabled,
		EnvName:            envName,
		MultiTenantURL:     url,
	}

	handler, _, _, cleanup, err := initTenantMiddleware(cfg, &libLog.NopLogger{})
	if cleanup != nil {
		defer cleanup()
	}

	return handler, err
}
