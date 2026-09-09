// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// metadataRejectionRequest is a request that passes every other check, so the
// only rejection a subtest can trigger is the metadata one it sets up.
func metadataRejectionRequest() *model.ValidationRequest {
	return &model.ValidationRequest{
		RequestID:            uuid.New(),
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString("10"),
		Asset:                "USD",
		TransactionTimestamp: time.Now().UTC(),
		Account: model.AccountContext{
			ID:     uuid.New(),
			Type:   "checking",
			Status: "active",
		},
	}
}

// TestMetadataKeyRejectionNamesTheKeyAndTheCeiling pins that a client sending a
// metadata key that is too long is told which key to shorten and how short it
// has to be. The rejection used to carry Go format markers where both belonged,
// so a client could not tell which of its keys the API objected to.
func TestMetadataKeyRejectionNamesTheKeyAndTheCeiling(t *testing.T) {
	longKey := strings.Repeat("k", trcConstant.MaxMetadataKeyLength+1)

	request := metadataRejectionRequest()
	request.Metadata = map[string]any{longKey: "value"}

	err := request.NormalizeAndValidate(time.Now().UTC())
	require.Error(t, err)
	require.ErrorIs(t, err, constant.ErrMetadataKeyLengthExceeded,
		"the rejection must still match the shared sentinel")

	for _, entityType := range []string{constant.EntityValidationRequest, constant.EntityReservation} {
		t.Run(entityType, func(t *testing.T) {
			body, marshalErr := json.Marshal(renderRequestRejection(err, entityType))
			require.NoError(t, marshalErr)

			rendered := string(body)

			assert.NotContains(t, rendered, "MISSING",
				"the rejection carries Go format markers instead of the key and the ceiling: %s", rendered)
			assert.Contains(t, rendered, longKey,
				"the rejection does not name the key the client must shorten")
			assert.Contains(t, rendered, strconv.Itoa(trcConstant.MaxMetadataKeyLength),
				"the rejection does not name the length ceiling")
			assert.Contains(t, rendered, constant.ErrMetadataKeyLengthExceeded.Error(),
				"the rejection changed error code")
		})
	}
}

// TestOtherRejectionsAreUnchanged pins that only the key-length rejection takes
// the named path: every other validation failure still renders through the
// shared registry exactly as before.
func TestOtherRejectionsAreUnchanged(t *testing.T) {
	metadata := make(map[string]any, trcConstant.MaxMetadataEntries+1)
	for i := range trcConstant.MaxMetadataEntries + 1 {
		metadata["k"+strconv.Itoa(i)] = "v"
	}

	request := metadataRejectionRequest()
	request.Metadata = metadata

	err := request.NormalizeAndValidate(time.Now().UTC())
	require.ErrorIs(t, err, constant.ErrMetadataEntriesExceeded)

	body, marshalErr := json.Marshal(renderRequestRejection(err, constant.EntityValidationRequest))
	require.NoError(t, marshalErr)

	rendered := string(body)
	assert.NotContains(t, rendered, "MISSING")
	assert.Contains(t, rendered, constant.ErrMetadataEntriesExceeded.Error())
}
