// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/pkg"
)

// assertBusinessCode asserts that err is a canonical pkg typed business error
// carrying the given code. pkg.ValidateBusinessError returns typed structs
// (EntityNotFoundError, ValidationError, UnprocessableOperationError, ...) that
// do NOT wrap the originating sentinel, so errors.Is against the sentinel does
// not match — the durable assertion is on the typed error's Code field.
func assertBusinessCode(t *testing.T, err error, code string) {
	t.Helper()

	if err == nil {
		assert.Fail(t, "expected a business error, got nil")
		return
	}

	var got string

	switch e := err.(type) {
	case pkg.EntityNotFoundError:
		got = e.Code
	case pkg.ValidationError:
		got = e.Code
	case pkg.EntityConflictError:
		got = e.Code
	case pkg.UnprocessableOperationError:
		got = e.Code
	case pkg.UnauthorizedError:
		got = e.Code
	case pkg.ForbiddenError:
		got = e.Code
	case pkg.InternalServerError:
		got = e.Code
	case pkg.FailedPreconditionError:
		got = e.Code
	case pkg.ServiceUnavailableError:
		got = e.Code
	default:
		assert.Failf(t, "unexpected error type", "got %T: %v", err, err)
		return
	}

	assert.Equal(t, code, got, "business error must carry the canonical code")
}
