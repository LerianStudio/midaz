// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"encoding/json"
	"maps"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// FlattenLegacyFeeExemption returns a feeExemption stored as an object (written before v4.1.1)
// as the JSON string the fee engine writes. The input map is never modified.
func FlattenLegacyFeeExemption(metadata map[string]any) map[string]any {
	var exemption map[string]any

	switch value := metadata["feeExemption"].(type) {
	case map[string]any:
		exemption = value
	case bson.D:
		exemption = make(map[string]any, len(value))
		for _, element := range value {
			exemption[element.Key] = element.Value
		}
	default:
		return metadata
	}

	encoded, err := json.Marshal(exemption)
	if err != nil {
		return metadata
	}

	flat := maps.Clone(metadata)
	flat["feeExemption"] = string(encoded)

	return flat
}
