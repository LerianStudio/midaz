// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"fmt"

	"github.com/tink-crypto/tink-go/v2/keyset"
	"github.com/tink-crypto/tink-go/v2/tink"
)

// MACPrimitive wraps a Tink MAC primitive for computing message authentication codes.
// Retained for the legacy read path (LegacyMACPrimitive); search tokens use the PRF
// primitives in prf.go.
type MACPrimitive struct {
	primitive tink.MAC
}

// ComputeMAC computes a MAC tag for the given data.
// The output is deterministic: same data always produces the same tag.
func (m *MACPrimitive) ComputeMAC(data []byte) ([]byte, error) {
	tag, err := m.primitive.ComputeMAC(data)
	if err != nil {
		return nil, fmt.Errorf("MAC computation failed: %w", err)
	}

	return tag, nil
}

// createSingleKeyHandle creates a keyset handle containing only the specified key entry.
func createSingleKeyHandle(entry *keyset.Entry) (*keyset.Handle, error) {
	manager := keyset.NewManager()

	keyID, err := manager.AddKey(entry.Key())
	if err != nil {
		return nil, fmt.Errorf("failed to add key to manager: %w", err)
	}

	if err := manager.SetPrimary(keyID); err != nil {
		return nil, fmt.Errorf("failed to set primary key: %w", err)
	}

	return manager.Handle()
}
