// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libCrypto "github.com/LerianStudio/lib-commons/v2/commons/crypto"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// brokenCrypto returns a *libCrypto.Crypto whose Cipher is nil, so every
// Encrypt/Decrypt call on a non-nil plaintext returns "cipher not initialized".
// This exercises the error-wrap branches in the holder field-level mappers
// without mocking anything: we are driving the real mapper code against the
// real (uninitialized) crypto type.
//
// Nil-pointer plaintexts short-circuit with (nil, nil) inside lib-commons, so
// we can selectively toggle which branch fails by setting only one field.
func brokenCrypto() *libCrypto.Crypto {
	return &libCrypto.Crypto{
		// Cipher intentionally left nil.
		Logger: &libLog.GoLogger{Level: libLog.InfoLevel},
	}
}

func strPtr(s string) *string { return &s }

// -- mapContactFromEntity error branches --------.

func TestMapContactFromEntity_EncryptPrimaryEmailError(t *testing.T) {
	t.Parallel()

	got, err := mapContactFromEntity(brokenCrypto(), &mmodel.Contact{
		PrimaryEmail: strPtr("user@example.com"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "encrypt primary email")
}

func TestMapContactFromEntity_EncryptSecondaryEmailError(t *testing.T) {
	t.Parallel()

	// PrimaryEmail nil -> Encrypt(nil) returns (nil, nil); secondary then fails.
	got, err := mapContactFromEntity(brokenCrypto(), &mmodel.Contact{
		SecondaryEmail: strPtr("alt@example.com"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "encrypt secondary email")
}

func TestMapContactFromEntity_EncryptMobilePhoneError(t *testing.T) {
	t.Parallel()

	got, err := mapContactFromEntity(brokenCrypto(), &mmodel.Contact{
		MobilePhone: strPtr("+15555555555"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "encrypt mobile phone")
}

func TestMapContactFromEntity_EncryptOtherPhoneError(t *testing.T) {
	t.Parallel()

	got, err := mapContactFromEntity(brokenCrypto(), &mmodel.Contact{
		OtherPhone: strPtr("+15555550000"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "encrypt other phone")
}

// -- mapContactToEntity error branches --------.

func TestMapContactToEntity_DecryptPrimaryEmailError(t *testing.T) {
	t.Parallel()

	got, err := mapContactToEntity(brokenCrypto(), &ContactMongoDBModel{
		PrimaryEmail: strPtr("ciphertext"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decrypt primary email")
}

func TestMapContactToEntity_DecryptSecondaryEmailError(t *testing.T) {
	t.Parallel()

	got, err := mapContactToEntity(brokenCrypto(), &ContactMongoDBModel{
		SecondaryEmail: strPtr("ciphertext"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decrypt secondary email")
}

func TestMapContactToEntity_DecryptMobilePhoneError(t *testing.T) {
	t.Parallel()

	got, err := mapContactToEntity(brokenCrypto(), &ContactMongoDBModel{
		MobilePhone: strPtr("ciphertext"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decrypt mobile phone")
}

func TestMapContactToEntity_DecryptOtherPhoneError(t *testing.T) {
	t.Parallel()

	got, err := mapContactToEntity(brokenCrypto(), &ContactMongoDBModel{
		OtherPhone: strPtr("ciphertext"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decrypt other phone")
}

// -- mapNaturalPersonFromEntity error branches --------.

func TestMapNaturalPersonFromEntity_EncryptMotherNameError(t *testing.T) {
	t.Parallel()

	got, err := mapNaturalPersonFromEntity(brokenCrypto(), &mmodel.NaturalPerson{
		MotherName: strPtr("Jane"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "encrypt mother name")
}

func TestMapNaturalPersonFromEntity_EncryptFatherNameError(t *testing.T) {
	t.Parallel()

	got, err := mapNaturalPersonFromEntity(brokenCrypto(), &mmodel.NaturalPerson{
		FatherName: strPtr("John"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "encrypt father name")
}

// -- mapNaturalPersonToEntity error branches --------.

func TestMapNaturalPersonToEntity_DecryptMotherNameError(t *testing.T) {
	t.Parallel()

	got, err := mapNaturalPersonToEntity(brokenCrypto(), &NaturalPersonMongoDBModel{
		MotherName: strPtr("ciphertext"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decrypt mother name")
}

func TestMapNaturalPersonToEntity_DecryptFatherNameError(t *testing.T) {
	t.Parallel()

	got, err := mapNaturalPersonToEntity(brokenCrypto(), &NaturalPersonMongoDBModel{
		FatherName: strPtr("ciphertext"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decrypt father name")
}

// -- mapRepresentativeToEntity error branches --------.

func TestMapRepresentativeToEntity_DecryptNameError(t *testing.T) {
	t.Parallel()

	got, err := mapRepresentativeToEntity(brokenCrypto(), &RepresentativeMongoDBModel{
		Name: strPtr("ciphertext"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decrypt representative name")
}

func TestMapRepresentativeToEntity_DecryptDocumentError(t *testing.T) {
	t.Parallel()

	got, err := mapRepresentativeToEntity(brokenCrypto(), &RepresentativeMongoDBModel{
		Document: strPtr("ciphertext"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decrypt representative document")
}

func TestMapRepresentativeToEntity_DecryptEmailError(t *testing.T) {
	t.Parallel()

	got, err := mapRepresentativeToEntity(brokenCrypto(), &RepresentativeMongoDBModel{
		Email: strPtr("ciphertext"),
	})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "decrypt representative email")
}
