// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"

	"github.com/gofiber/fiber/v3"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

func contextCompletionAuthorization(legacy fiber.Handler) fiber.Handler {
	return func(c fiber.Ctx) error {
		if _, ok := contextutil.GetIntegrationIdentity(c.Context()); !ok {
			return pkgHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrInsufficientPrivileges, constant.EntityReservation))
		}

		if len(bytes.TrimSpace(c.Body())) == 0 {
			return legacy(c)
		}

		return c.Next()
	}
}
