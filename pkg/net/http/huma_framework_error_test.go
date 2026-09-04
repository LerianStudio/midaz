// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"net/http"
	"testing"

	libProblem "github.com/LerianStudio/lib-commons/v7/commons/net/http/problem"
	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// TestInstallHumaFrameworkErrors_EmptyBodyMapsTo0094 asserts the empty-body
// precondition lands on the canonical malformed-body envelope: the same code,
// title, status and type any other unparseable body gets from HumaProblem.
func TestInstallHumaFrameworkErrors_EmptyBodyMapsTo0094(t *testing.T) {
	// NOT parallel: assigns the process-global huma.NewErrorWithContext.
	libProblem.Install()
	InstallHumaFrameworkErrors()

	err := huma.NewErrorWithContext(nil, http.StatusBadRequest, humaEmptyBodyMessage)

	detail, ok := err.(*Detail)
	require.True(t, ok, "empty-body error must be the Midaz *Detail, got %T", err)

	assert.Equal(t, http.StatusBadRequest, detail.GetStatus())
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), detail.Code, "empty body carries the malformed-body code")
	assert.Equal(t, "Unmarshalling error", detail.Title)
	assert.Equal(t, "unexpected end of JSON input", detail.Detail.Detail)
	assert.Equal(t, libProblem.BaseURI+"/"+constant.ErrInvalidRequestBody.Error(), detail.Type)
}

// TestInstallHumaFrameworkErrors_OtherErrorsDelegate asserts every other framework
// error still flows through huma.NewError, so lib-commons' problem.Install override
// keeps owning them (problem+json shape, no business code).
func TestInstallHumaFrameworkErrors_OtherErrorsDelegate(t *testing.T) {
	// NOT parallel: assigns the process-global huma.NewErrorWithContext.
	libProblem.Install()
	InstallHumaFrameworkErrors()

	tests := []struct {
		name   string
		status int
		msg    string
	}{
		{"not found", http.StatusNotFound, "not found"},
		{"validation failed", http.StatusUnprocessableEntity, "validation failed"},
		{"bad request other than empty body", http.StatusBadRequest, "invalid parameter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := huma.NewErrorWithContext(nil, tt.status, tt.msg)

			assert.Equal(t, tt.status, err.GetStatus())

			_, isMidazDetail := err.(*Detail)
			assert.False(t, isMidazDetail, "non-empty-body errors must not be remapped to the 0094 envelope")

			// Must be the lib-commons fallback type, not merely "not ours": that is
			// what proves delegation to huma.NewError actually happened.
			commonsDetail, ok := err.(*libProblem.Detail)
			require.True(t, ok, "must delegate to huma.NewError's *problem.Detail, got %T", err)
			assert.Empty(t, commonsDetail.Code, "framework errors carry no business code")
		})
	}
}
