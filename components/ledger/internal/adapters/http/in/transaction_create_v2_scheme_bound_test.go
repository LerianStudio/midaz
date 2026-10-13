// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/scheme"
)

// TestCreateTransactionV2Request_SchemeBoundMatchesSharedRule locks the scheme field to
// the shared scheme rule: the validate tag is what rejects a body, and the maxLength tag
// is what the published schema advertises, so both must agree with pkg/scheme.
func TestCreateTransactionV2Request_SchemeBoundMatchesSharedRule(t *testing.T) {
	t.Parallel()

	field, ok := reflect.TypeOf(CreateTransactionV2Request{}).FieldByName("Scheme")
	require.True(t, ok, "CreateTransactionV2Request must carry a Scheme field")

	maxLength, err := strconv.Atoi(field.Tag.Get("maxLength"))
	require.NoError(t, err, "the Scheme maxLength tag must be an integer")
	assert.Equal(t, scheme.MaxLength, maxLength,
		"the published maxLength must equal the shared scheme bound")

	assert.Contains(t, strings.Split(field.Tag.Get("validate"), ","), "scheme",
		"the Scheme validate tag must enforce the shared scheme rule")
	assert.Empty(t, field.Tag.Get("enum"), "the Scheme field must not publish a closed enum")
}
