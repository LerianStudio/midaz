// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tink

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/tink-crypto/tink-go/v2/aead"
	"github.com/tink-crypto/tink-go/v2/insecurecleartextkeyset"
	"github.com/tink-crypto/tink-go/v2/keyset"
	"github.com/tink-crypto/tink-go/v2/mac"
	aesgcmpb "github.com/tink-crypto/tink-go/v2/proto/aes_gcm_go_proto"
	commonpb "github.com/tink-crypto/tink-go/v2/proto/common_go_proto"
	macpb "github.com/tink-crypto/tink-go/v2/proto/hmac_go_proto"
	tinkpb "github.com/tink-crypto/tink-go/v2/proto/tink_go_proto"
	"google.golang.org/protobuf/proto"
)

const legacyImportedKeyID uint32 = 1

// LegacyMACPrimitive wraps a MAC primitive for computing legacy-compatible HMAC tokens.
// It produces lowercase hex-encoded HMAC-SHA256 tags matching lib-commons GenerateHash output.
type LegacyMACPrimitive struct {
	primitive *MACPrimitive
}

// NewLegacyAESGCMPrimitiveFromHexKey creates a Tink AEAD primitive from a hex-encoded AES key.
// The primitive uses RAW output prefix to match lib-commons AES-GCM format (nonce || ciphertext || tag).
// Key must be 16 (AES-128), 24 (AES-192), or 32 (AES-256) bytes when decoded.
func NewLegacyAESGCMPrimitiveFromHexKey(hexKey string) (*AEADPrimitive, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("decode legacy AES-GCM key: %w", err)
	}

	switch len(key) {
	case 16, 24, 32:
	default:
		return nil, fmt.Errorf("legacy AES-GCM key must be 16, 24, or 32 bytes: got %d", len(key))
	}

	keyData, err := proto.Marshal(&aesgcmpb.AesGcmKey{Version: 0, KeyValue: key})
	if err != nil {
		return nil, fmt.Errorf("marshal legacy AES-GCM key: %w", err)
	}

	handle, err := readLegacyKeyset(&tinkpb.Keyset{
		PrimaryKeyId: legacyImportedKeyID,
		Key: []*tinkpb.Keyset_Key{{
			KeyData: &tinkpb.KeyData{
				TypeUrl:         "type.googleapis.com/google.crypto.tink.AesGcmKey",
				Value:           keyData,
				KeyMaterialType: tinkpb.KeyData_SYMMETRIC,
			},
			Status:           tinkpb.KeyStatusType_ENABLED,
			KeyId:            legacyImportedKeyID,
			OutputPrefixType: tinkpb.OutputPrefixType_RAW,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("read legacy AES-GCM keyset: %w", err)
	}

	primitive, err := aead.New(handle)
	if err != nil {
		return nil, fmt.Errorf("create legacy AES-GCM primitive: %w", err)
	}

	return &AEADPrimitive{primitive: primitive}, nil
}

// NewLegacyMACPrimitiveFromSecret creates a MAC primitive from a plain string secret.
// The primitive produces HMAC-SHA256 tags compatible with lib-commons GenerateHash.
func NewLegacyMACPrimitiveFromSecret(secret string) (*LegacyMACPrimitive, error) {
	if secret == "" {
		return nil, fmt.Errorf("legacy HMAC key must not be empty")
	}

	keyData, err := proto.Marshal(&macpb.HmacKey{
		Version:  0,
		KeyValue: []byte(secret),
		Params: &macpb.HmacParams{
			Hash:    commonpb.HashType_SHA256,
			TagSize: 32,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal legacy HMAC key: %w", err)
	}

	handle, err := readLegacyKeyset(&tinkpb.Keyset{
		PrimaryKeyId: legacyImportedKeyID,
		Key: []*tinkpb.Keyset_Key{{
			KeyData: &tinkpb.KeyData{
				TypeUrl:         "type.googleapis.com/google.crypto.tink.HmacKey",
				Value:           keyData,
				KeyMaterialType: tinkpb.KeyData_SYMMETRIC,
			},
			Status:           tinkpb.KeyStatusType_ENABLED,
			KeyId:            legacyImportedKeyID,
			OutputPrefixType: tinkpb.OutputPrefixType_RAW,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("read legacy HMAC keyset: %w", err)
	}

	primitive, err := mac.New(handle)
	if err != nil {
		return nil, fmt.Errorf("create legacy HMAC primitive: %w", err)
	}

	return &LegacyMACPrimitive{primitive: &MACPrimitive{primitive: primitive}}, nil
}

// ComputeLegacyHexToken computes an HMAC-SHA256 tag and returns it as lowercase hex.
// This matches the output format of lib-commons GenerateHash for migration compatibility.
func (m *LegacyMACPrimitive) ComputeLegacyHexToken(data []byte) (string, error) {
	if m == nil || m.primitive == nil {
		return "", fmt.Errorf("legacy HMAC primitive is not configured")
	}

	tag, err := m.primitive.ComputeMAC(data)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(tag), nil
}

// readLegacyKeyset deserializes a Tink keyset proto into a handle.
// Used internally for importing raw key material with specific output prefix types.
func readLegacyKeyset(ks *tinkpb.Keyset) (*keyset.Handle, error) {
	data, err := proto.Marshal(ks)
	if err != nil {
		return nil, fmt.Errorf("marshal legacy keyset: %w", err)
	}

	handle, err := insecurecleartextkeyset.Read(keyset.NewBinaryReader(bytes.NewReader(data)))
	if err != nil {
		return nil, fmt.Errorf("read legacy keyset: %w", err)
	}

	return handle, nil
}
