// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ErrTracerRequestRejected marks a request the Tracer refused before
// evaluating it and named with a canonical code: an unauthorized producer or
// tenant (0043), a missing tenant (0487) or a missing policy (0527). The
// Tracer then holds nothing for the transaction, and redelivering the same
// request cannot change the answer. The canonical cause is wrapped alongside
// it. A refusal without a recognized code is never this error.
var ErrTracerRequestRejected = errors.New("tracer rejected the request before evaluation")

// seamDeterministicCauses are the canonical codes both transports recognize as
// a deterministic outcome of the request rather than a service outage.
var seamDeterministicCauses = []error{
	constant.ErrContextLimitsUnavailable,
	constant.ErrExpressionCostExceeded,
	constant.ErrExpressionEvaluation,
	constant.ErrInvalidRequestBody,
	constant.ErrPayloadTooLarge,
	constant.ErrReserveOperationConflict,
	constant.ErrTracerContractUnavailable,
}

// seamRejectionCauses are the canonical codes of a refusal before evaluation.
var seamRejectionCauses = []error{
	constant.ErrInsufficientPrivileges,
	constant.ErrReservationTenantRequired,
	constant.ErrContextPolicyUnavailable,
}

// seamCause maps a canonical code to its seam error, or nil when code is not
// one the seam recognizes. Remote error text is never copied.
func seamCause(code string) error {
	for _, cause := range seamRejectionCauses {
		if code == cause.Error() {
			return rejectedBy(cause)
		}
	}

	for _, cause := range seamDeterministicCauses {
		if code == cause.Error() {
			return cause
		}
	}

	return nil
}

func rejectedBy(cause error) error {
	return fmt.Errorf("%w: %w", ErrTracerRequestRejected, cause)
}
