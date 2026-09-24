// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestSharedEvaluationSchemaNames(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[tracercontract.Account](), reflect.TypeFor[*tracercontract.Account]()} {
		require.Equal(t, "EvaluationAccount", sharedSchemaNamer(typ, ""))
		require.Equal(t, "EvaluationAccount", ledgerSchemaNamer(typ, ""))
	}
	require.Equal(t, "AccountV2", ledgerSchemaNamer(reflect.TypeFor[mmodel.Account](), ""))
}
