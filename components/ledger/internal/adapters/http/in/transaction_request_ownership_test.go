// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTransactionRequestsBelongToInboundHTTPAdapter prevents create payloads from drifting
// back into a root pkg package. Their historical OpenAPI names are protected separately by
// the typed request-body and unified contract-document tests.
func TestTransactionRequestsBelongToInboundHTTPAdapter(t *testing.T) {
	t.Parallel()

	const wantPackage = "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"

	requestTypes := []reflect.Type{
		reflect.TypeFor[CreateTransactionRequest](),
		reflect.TypeFor[CreateTransactionInflowRequestBody](),
		reflect.TypeFor[TransactionInflowSendRequest](),
		reflect.TypeFor[CreateTransactionOutflowRequestBody](),
		reflect.TypeFor[TransactionOutflowSendRequest](),
		reflect.TypeFor[CreateTransactionV2Request](),
		reflect.TypeFor[TransactionV2LegRequest](),
		reflect.TypeFor[TransactionV2ShareRequest](),
	}

	for _, requestType := range requestTypes {
		require.Equalf(t, wantPackage, requestType.PkgPath(), "%s must be owned by the inbound HTTP adapter", requestType.Name())
	}
}

// TestTransactionRequestSchemaNamesRemainStable pins the public names independently from the
// Request-oriented Go names. Moving ownership must not churn generated clients.
func TestTransactionRequestSchemaNamesRemainStable(t *testing.T) {
	t.Parallel()

	_, api := buildUnifiedHumaAPI()
	registry := api.OpenAPI().Components.Schemas

	wantNames := map[reflect.Type]string{
		reflect.TypeFor[CreateTransactionRequest]():            "CreateTransactionInput",
		reflect.TypeFor[CreateTransactionInflowRequestBody]():  "CreateTransactionInflowInput",
		reflect.TypeFor[TransactionInflowSendRequest]():        "SendInflow",
		reflect.TypeFor[CreateTransactionOutflowRequestBody](): "CreateTransactionOutflowInput",
		reflect.TypeFor[TransactionOutflowSendRequest]():       "SendOutflow",
		reflect.TypeFor[CreateTransactionV2Request]():          "CreateTransactionV2Input",
		reflect.TypeFor[TransactionV2LegRequest]():             "V2LegInput",
		reflect.TypeFor[TransactionV2ShareRequest]():           "V2ShareInput",
	}

	for requestType, wantName := range wantNames {
		gotRef := registry.Schema(requestType, true, "").Ref
		require.Equalf(t, "#/components/schemas/"+wantName, gotRef, "%s must retain its published schema name", requestType.Name())
	}
}
