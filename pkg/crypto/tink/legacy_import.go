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
	"github.com/tink-crypto/tink-go/v2/prf"
	aesgcmpb "github.com/tink-crypto/tink-go/v2/proto/aes_gcm_go_proto"
	commonpb "github.com/tink-crypto/tink-go/v2/proto/common_go_proto"
	hmacprfpb "github.com/tink-crypto/tink-go/v2/proto/hmac_prf_go_proto"
	tinkpb "github.com/tink-crypto/tink-go/v2/proto/tink_go_proto"
	"google.golang.org/protobuf/proto"
)

const legacyImportedKeyID uint32 = 1

// legacyComposedKeyID is the fixed key ID assigned to the imported legacy key
// when it is composed as a NON-primary entry alongside a freshly generated
// primary key. It is chosen to never collide with Tink's randomly generated
// fresh key IDs (collision is rejected at compose time as a defensive guard).
const legacyComposedKeyID uint32 = 0xFFFFFFFF

const (
	legacyAESGCMTypeURL  = "type.googleapis.com/google.crypto.tink.AesGcmKey"
	legacyHMACPRFTypeURL = "type.googleapis.com/google.crypto.tink.HmacPrfKey"
)

// LegacyPRFPrimitive wraps a Tink HMAC-PRF set for computing legacy-compatible
// HMAC tokens. At a 32-byte output length, Tink HMAC-SHA256-PRF equals the full
// HMAC-SHA256 tag, so the hex-encoded output is byte-identical to lib-commons
// GenerateHash. Used for the legacy hash-search read path during migration.
type LegacyPRFPrimitive struct {
	set *prf.Set
}

// NewLegacyAESGCMPrimitiveFromHexKey creates a Tink AEAD primitive from a hex-encoded AES key.
// The primitive uses RAW output prefix to match lib-commons AES-GCM format (nonce || ciphertext || tag).
// Key must be 16 (AES-128), 24 (AES-192), or 32 (AES-256) bytes when decoded.
func NewLegacyAESGCMPrimitiveFromHexKey(hexKey string) (*AEADPrimitive, error) {
	legacyKey, err := legacyAESGCMKeysetKey(hexKey, legacyImportedKeyID)
	if err != nil {
		return nil, err
	}

	handle, err := readLegacyKeyset(&tinkpb.Keyset{
		PrimaryKeyId: legacyImportedKeyID,
		Key:          []*tinkpb.Keyset_Key{legacyKey},
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

// NewLegacyPRFPrimitiveFromSecret creates an HMAC-PRF primitive from a plain
// string secret. The primitive produces 32-byte HMAC-SHA256 outputs compatible
// with lib-commons GenerateHash. PRF keys are always RAW (no key-id prefix).
func NewLegacyPRFPrimitiveFromSecret(secret string) (*LegacyPRFPrimitive, error) {
	legacyKey, err := legacyHMACPRFKeysetKey(secret, legacyImportedKeyID)
	if err != nil {
		return nil, err
	}

	handle, err := readLegacyKeyset(&tinkpb.Keyset{
		PrimaryKeyId: legacyImportedKeyID,
		Key:          []*tinkpb.Keyset_Key{legacyKey},
	})
	if err != nil {
		return nil, fmt.Errorf("read legacy HMAC-PRF keyset: %w", err)
	}

	set, err := prf.NewPRFSet(handle)
	if err != nil {
		return nil, fmt.Errorf("create legacy HMAC-PRF primitive: %w", err)
	}

	return &LegacyPRFPrimitive{set: set}, nil
}

// ComputeLegacyHexToken computes a 32-byte HMAC-SHA256 PRF output and returns it
// as lowercase hex. This matches the output format of lib-commons GenerateHash
// for migration compatibility.
func (m *LegacyPRFPrimitive) ComputeLegacyHexToken(data []byte) (string, error) {
	if m == nil || m.set == nil {
		return "", fmt.Errorf("legacy HMAC-PRF primitive is not configured")
	}

	out, err := m.set.ComputePrimaryPRF(data, searchTokenPRFOutputBytes)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(out), nil
}

// legacyAESGCMKeysetKey builds an ENABLED, RAW-prefix Tink keyset key entry from
// a hex-encoded legacy AES-GCM key, using the given key ID. It is the shared
// building block for both the single-key legacy primitive and the composite
// (mixed) keyset. It never returns or logs the raw key material.
func legacyAESGCMKeysetKey(hexKey string, keyID uint32) (*tinkpb.Keyset_Key, error) {
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

	return &tinkpb.Keyset_Key{
		KeyData: &tinkpb.KeyData{
			TypeUrl:         legacyAESGCMTypeURL,
			Value:           keyData,
			KeyMaterialType: tinkpb.KeyData_SYMMETRIC,
		},
		Status:           tinkpb.KeyStatusType_ENABLED,
		KeyId:            keyID,
		OutputPrefixType: tinkpb.OutputPrefixType_RAW,
	}, nil
}

// legacyHMACPRFKeysetKey builds an ENABLED, RAW-prefix Tink HMAC-PRF keyset key
// entry from a plain string secret, using the given key ID. Shared by the
// single-key legacy primitive and the composite (mixed) keyset.
func legacyHMACPRFKeysetKey(secret string, keyID uint32) (*tinkpb.Keyset_Key, error) {
	if secret == "" {
		return nil, fmt.Errorf("legacy HMAC key must not be empty")
	}

	keyData, err := proto.Marshal(&hmacprfpb.HmacPrfKey{
		Version:  0,
		KeyValue: []byte(secret),
		Params: &hmacprfpb.HmacPrfParams{
			Hash: commonpb.HashType_SHA256,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal legacy HMAC-PRF key: %w", err)
	}

	return &tinkpb.Keyset_Key{
		KeyData: &tinkpb.KeyData{
			TypeUrl:         legacyHMACPRFTypeURL,
			Value:           keyData,
			KeyMaterialType: tinkpb.KeyData_SYMMETRIC,
		},
		Status:           tinkpb.KeyStatusType_ENABLED,
		KeyId:            keyID,
		OutputPrefixType: tinkpb.OutputPrefixType_RAW,
	}, nil
}

// composeMixedKeyset builds a composite keyset that keeps the freshly generated
// key as PRIMARY and adds the imported legacy key as an ENABLED, NON-primary
// entry. The fresh keys are taken verbatim from freshHandle (already generated
// and serialized by the caller) and the legacy key is supplied as a prebuilt
// keyset key entry. It returns the serialized composite keyset bytes (CLEARTEXT
// key material; never persist) and the KeysetInfo describing both entries.
//
// purpose labels the fresh key's KeyType; legacyType labels the imported key.
// The fresh primary key ID is preserved as the composite primary so existing
// envelope/search behavior over the fresh key is unchanged.
func composeMixedKeyset(freshHandle *keyset.Handle, legacyKey *tinkpb.Keyset_Key, purpose keyPurpose, legacyType KeyType) ([]byte, KeysetInfo, error) {
	freshKeyset, err := keysetProtoFromHandle(freshHandle)
	if err != nil {
		return nil, KeysetInfo{}, err
	}

	if len(freshKeyset.GetKey()) == 0 {
		return nil, KeysetInfo{}, fmt.Errorf("fresh keyset has no keys")
	}

	primaryID := freshKeyset.GetPrimaryKeyId()

	// Defensive guard: the legacy composed key ID must not collide with any fresh
	// key ID. The fresh keyset here holds a single, freshly generated primary key
	// whose ID Tink assigns as a random 32-bit value; legacyComposedKeyID is the
	// fixed sentinel (0xFFFFFFFF). Collision is astronomically unlikely but the loop
	// rejects it anyway (and stays correct if a multi-key fresh keyset is ever used).
	for _, k := range freshKeyset.GetKey() {
		if k.GetKeyId() == legacyKey.GetKeyId() {
			return nil, KeysetInfo{}, fmt.Errorf("legacy key ID collides with a fresh key ID")
		}
	}

	composite := &tinkpb.Keyset{
		PrimaryKeyId: primaryID,
		Key:          append(freshKeyset.GetKey(), legacyKey),
	}

	// Validate the composite by reading it into a handle (also the form the caller
	// will later unwrap), then serialize via the shared cleartext writer.
	handle, err := readLegacyKeyset(composite)
	if err != nil {
		return nil, KeysetInfo{}, fmt.Errorf("compose mixed keyset: %w", err)
	}

	serialized, err := serializeKeyset(handle)
	if err != nil {
		return nil, KeysetInfo{}, fmt.Errorf("serialize mixed keyset: %w", err)
	}

	// Derive the composite KeysetInfo from the validated handle via the shared
	// extractKeysetInfo (the same path used for fresh keysets), then relabel ONLY
	// the imported legacy entry's Type. extractKeysetInfo already sets IsPrimary
	// from the handle's PrimaryKeyId, so the legacy entry (non-primary) is correct.
	info, err := extractKeysetInfo(handle, purpose)
	if err != nil {
		return nil, KeysetInfo{}, fmt.Errorf("extract mixed keyset info: %w", err)
	}

	relabelLegacyKeyType(&info, legacyKey.GetKeyId(), legacyType)

	return serialized, info, nil
}

// relabelLegacyKeyType overwrites the KeyType of the imported legacy entry
// (identified by legacyKeyID) with legacyType. The fresh keys keep the
// purpose-derived type assigned by extractKeysetInfo.
func relabelLegacyKeyType(info *KeysetInfo, legacyKeyID uint32, legacyType KeyType) {
	for i := range info.Keys {
		if info.Keys[i].KeyID == legacyKeyID {
			info.Keys[i].Type = legacyType
		}
	}
}

// keysetProtoFromHandle round-trips a fresh handle through the cleartext
// serializer to obtain its underlying Keyset proto. This reuses the existing
// fresh-key generation rather than re-deriving key material.
func keysetProtoFromHandle(handle *keyset.Handle) (*tinkpb.Keyset, error) {
	serialized, err := serializeKeyset(handle)
	if err != nil {
		return nil, fmt.Errorf("serialize fresh keyset: %w", err)
	}

	ks := &tinkpb.Keyset{}
	if err := proto.Unmarshal(serialized, ks); err != nil {
		return nil, fmt.Errorf("unmarshal fresh keyset: %w", err)
	}

	return ks, nil
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
