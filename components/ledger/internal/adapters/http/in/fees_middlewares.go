// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	feeerrors "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const (
	feeUUIDPathParameter    = "id"
	feeOrgIDHeaderParameter = "X-Organization-Id"
)

// parseFeePathParameters validates the fee/billing UUID path parameter and stores
// the parsed value in the request locals so fee handlers can read it as a uuid.UUID.
func parseFeePathParameters(c *fiber.Ctx) error {
	pathParam := c.Params(feeUUIDPathParameter)

	if commons.IsNilOrEmpty(&pathParam) {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidPathParameter, "", feeUUIDPathParameter))
	}

	parsedPathUUID, errPath := uuid.Parse(pathParam)
	if errPath != nil {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidPathParameter, "", feeUUIDPathParameter))
	}

	c.Locals(feeUUIDPathParameter, parsedPathUUID)

	return c.Next()
}

// parseFeeHeaderParameters validates the X-Organization-Id header and stores the
// parsed value in the request locals so fee handlers can read it as a uuid.UUID.
func parseFeeHeaderParameters(c *fiber.Ctx) error {
	headerParam := c.Get(feeOrgIDHeaderParameter)

	if commons.IsNilOrEmpty(&headerParam) {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrHeaderParameterRequired, "", feeOrgIDHeaderParameter))
	}

	parsedHeaderUUID, errHeader := uuid.Parse(headerParam)
	if errHeader != nil {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidHeaderParameter, "", feeOrgIDHeaderParameter))
	}

	c.Locals(feeOrgIDHeaderParameter, parsedHeaderUUID)

	return c.Next()
}
