// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	nethttp "net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestContextPolicyErrorContract(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		err      error
		status   int
		business bool
	}{
		{"missing configuration is unavailable, not a policy denial", constant.ErrContextPolicyUnavailable, nethttp.StatusServiceUnavailable, false},
		{"stale publication is a conflict", constant.ErrContextPolicyConflict, nethttp.StatusConflict, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typed := pkg.ValidateBusinessError(tc.err, constant.EntityRule)
			require.Equal(t, tc.business, pkg.IsBusinessError(typed))
			status, code := driveWithError(t, typed)
			require.Equal(t, tc.status, status)
			require.Equal(t, tc.err.Error(), code)
		})
	}
}
