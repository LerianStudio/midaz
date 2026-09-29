// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"encoding/json"
	"maps"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// flattenLegacyFeeExemption rewrites a feeExemption stored as an object to the JSON
// string the fee engine writes, so an older record builds a flat plan. Any other value,
// or one that cannot be encoded, is left for the flat-metadata check to judge.
func flattenLegacyFeeExemption(input mtransaction.Transaction) mtransaction.Transaction {
	var exemption map[string]any

	switch value := input.Metadata["feeExemption"].(type) {
	case map[string]any:
		exemption = value
	case bson.D:
		exemption = make(map[string]any, len(value))
		for _, element := range value {
			exemption[element.Key] = element.Value
		}
	default:
		return input
	}

	encoded, err := json.Marshal(exemption)
	if err != nil {
		return input
	}

	input.Metadata = maps.Clone(input.Metadata)
	input.Metadata["feeExemption"] = string(encoded)

	return input
}
