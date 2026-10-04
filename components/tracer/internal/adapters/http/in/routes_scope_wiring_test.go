// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"testing"

	"github.com/LerianStudio/lib-auth/v5/auth/declaration"
	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	"github.com/stretchr/testify/require"

	tracerembed "github.com/LerianStudio/midaz/v4/components/tracer"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/scoperesolver"
)

// wireTracerScope wires the auth client the way the boot does: the scope
// resolvers over the validations given, then the embedded manifest's scope.
func wireTracerScope(t *testing.T, authClient *authMiddleware.AuthClient, validations storedValidations) {
	t.Helper()

	require.NoError(t, scoperesolver.Register(authClient, validations, nil))
	require.NoError(t, declaration.WireScope(authClient, tracerembed.TracerManifest),
		"the boot scope wiring must accept the embedded manifest")
}
