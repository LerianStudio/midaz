// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestProvisionEncryptionResponse_JSONWireContract pins the JSON wire contract
// for the search-token primary key field. The field was renamed from
// "mac_primary_key_id" to "prf_primary_key_id"; this asserts the new name is
// emitted and the old name is gone.
func TestProvisionEncryptionResponse_JSONWireContract(t *testing.T) {
	resp := ProvisionEncryptionResponse{
		OrganizationID:   "00000000-0000-0000-0000-000000000000",
		KEKPath:          "transit/keys/org-00000000-0000-0000-0000-000000000000",
		AEADPrimaryKeyID: 1,
		PRFPrimaryKeyID:  1,
		Status:           "active",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	payload := string(data)

	if !strings.Contains(payload, `"prf_primary_key_id"`) {
		t.Errorf("expected payload to contain \"prf_primary_key_id\", got: %s", payload)
	}

	if strings.Contains(payload, `"mac_primary_key_id"`) {
		t.Errorf("expected payload to NOT contain \"mac_primary_key_id\", got: %s", payload)
	}
}
