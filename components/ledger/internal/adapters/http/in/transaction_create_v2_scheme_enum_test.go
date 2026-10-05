// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tracermodel "github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// TestCreateTransactionV2Request_SchemeEnumMatchesTracer locks the scheme enum the
// v2 create enforces to the tracer's TransactionType: a scheme the request admits is
// forwarded verbatim as the reserve's transaction type, so a value on one side and
// not the other makes the tracer refuse a request the ledger already validated, or
// the ledger refuse one the tracer would price. The validate tag is what rejects a
// body and the enum tag is what the published schema advertises, so both are read.
func TestCreateTransactionV2Request_SchemeEnumMatchesTracer(t *testing.T) {
	t.Parallel()

	field, ok := reflect.TypeOf(CreateTransactionV2Request{}).FieldByName("Scheme")
	require.True(t, ok, "CreateTransactionV2Request must carry a Scheme field")

	validateEnum := schemeOneOfValues(t, field.Tag.Get("validate"))
	schemaEnum := toSet(strings.Split(field.Tag.Get("enum"), ","))

	require.NotEmpty(t, schemaEnum, "the Scheme enum tag must list the accepted values")
	assert.Equal(t, validateEnum, schemaEnum,
		"the enforced oneof= list and the published enum tag must be identical")

	tracerTypes := []tracermodel.TransactionType{
		tracermodel.TransactionTypeCard,
		tracermodel.TransactionTypeWire,
		tracermodel.TransactionTypePix,
		tracermodel.TransactionTypeCrypto,
	}

	tracerEnum := make(map[string]struct{}, len(tracerTypes))

	for _, tt := range tracerTypes {
		require.Truef(t, tt.IsValid(), "tracer constant %q must be valid on the tracer side", tt)
		tracerEnum[string(tt)] = struct{}{}
	}

	assert.Equal(t, tracerEnum, validateEnum,
		"the request scheme enum and the tracer TransactionType set must be identical")

	const outsider = "TED"

	assert.False(t, tracermodel.TransactionType(outsider).IsValid(), "the tracer must refuse %q", outsider)
	assert.NotContains(t, validateEnum, outsider, "the request contract must refuse %q", outsider)
	assert.NotContains(t, schemaEnum, outsider, "the published schema must not advertise %q", outsider)
}

// schemeOneOfValues extracts the value set of the oneof= rule from a validate tag.
func schemeOneOfValues(t *testing.T, tag string) map[string]struct{} {
	t.Helper()

	var values []string

	for _, rule := range strings.Split(tag, ",") {
		if raw, found := strings.CutPrefix(rule, "oneof="); found {
			values = strings.Fields(raw)
		}
	}

	require.NotEmpty(t, values, "the Scheme validate tag must carry a oneof= rule")

	return toSet(values)
}

func toSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))

	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			set[v] = struct{}{}
		}
	}

	return set
}
