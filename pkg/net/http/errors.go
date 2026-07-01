// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"errors"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/gofiber/fiber/v2"
)

// WithError serializes err as the RFC 9457 problem+json envelope (Detail).
//
// The (code, HTTP status) tuple is the money path and is preserved byte-for-byte
// from the legacy {code,title,message} envelope: codeOf/statusOf inside
// withProblem reproduce WithError's original errors.As cascade in the SAME
// declaration order (r3 §2.1), and lib-commons MapError carries the code through
// verbatim. Only the envelope SHAPE changes (message->detail, +type, +errors[]),
// plus the deliberate >=500 title/detail scrub (r3 §2.3).
//
// Typed platform errors are resolved via errors.As so a wrapped error is still
// classified to its proper status. Business errors remain returned unwrapped by
// convention (E2); this is defensive hardening, not a license to wrap them.
// Resolution is order-dependent: the first matching arm wins, and because every
// platform error type has an Unwrap, errors.As walks the whole chain — so nesting
// platform errors makes the OUTERMOST class drive the status.
func WithError(c *fiber.Ctx, err error) error {
	// ResponseError (0094) is the status-in-Code quirk: its status is
	// strconv.Atoi(Code), not a table lookup, so it is dispatched on its own
	// branch BEFORE the code->status table (r3 §2.2).
	var responseErr pkg.ResponseError
	if errors.As(err, &responseErr) {
		return JSONResponseError(c, responseErr)
	}

	return withProblem(c, err)
}
