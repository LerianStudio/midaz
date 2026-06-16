// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package helpers

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/crypto/tink"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// TinkKeysetFixture contains real serialized Tink AEAD and PRF keysets plus
// their primary key IDs for tests that need envelope-encryption fixtures.
type TinkKeysetFixture struct {
	AEADBytes        []byte
	PRFBytes         []byte
	AEADPrimaryKeyID uint32
	PRFPrimaryKeyID  uint32
}

// GenerateTinkKeysets creates real Tink keysets for tests.
func GenerateTinkKeysets(t testing.TB) TinkKeysetFixture {
	t.Helper()

	aeadGen := tink.NewAEADKeysetGenerator()
	aeadHandle, aeadBytes, err := aeadGen.Generate()
	if err != nil {
		t.Fatalf("failed to generate AEAD keyset: %v", err)
	}

	aeadInfo, err := aeadGen.ExtractInfo(aeadHandle)
	if err != nil {
		t.Fatalf("failed to extract AEAD keyset info: %v", err)
	}

	prfGen := tink.NewPRFKeysetGenerator()
	prfHandle, prfBytes, err := prfGen.Generate()
	if err != nil {
		t.Fatalf("failed to generate PRF keyset: %v", err)
	}

	prfInfo, err := prfGen.ExtractInfo(prfHandle)
	if err != nil {
		t.Fatalf("failed to extract PRF keyset info: %v", err)
	}

	return TinkKeysetFixture{
		AEADBytes:        aeadBytes,
		PRFBytes:         prfBytes,
		AEADPrimaryKeyID: aeadInfo.PrimaryKeyID,
		PRFPrimaryKeyID:  prfInfo.PrimaryKeyID,
	}
}

// OrganizationKeysetFixtureInput customizes OrganizationKeysetFixture.
type OrganizationKeysetFixtureInput struct {
	TenantID          string
	OrganizationID    string
	KEKPath           string
	KEKMountPath      string
	WrappedKeyset     string
	WrappedHMACKeyset string
	AEADPrimaryKeyID  uint32
	HMACPrimaryKeyID  uint32
	Revision          int64
}

// OrganizationKeysetFixture returns a minimal organization keyset fixture using
// stable compatibility field names from the public storage model.
func OrganizationKeysetFixture(input OrganizationKeysetFixtureInput) *mmodel.OrganizationKeyset {
	return &mmodel.OrganizationKeyset{
		TenantID:          input.TenantID,
		OrganizationID:    input.OrganizationID,
		KEKPath:           input.KEKPath,
		KEKMountPath:      input.KEKMountPath,
		WrappedKeyset:     input.WrappedKeyset,
		KeysetInfo:        mmodel.KeysetInfo{PrimaryKeyID: input.AEADPrimaryKeyID},
		WrappedHMACKeyset: input.WrappedHMACKeyset,
		HMACKeysetInfo:    mmodel.KeysetInfo{PrimaryKeyID: input.HMACPrimaryKeyID},
		Revision:          input.Revision,
	}
}
