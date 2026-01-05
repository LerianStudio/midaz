package holder

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/tests/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMongoDBModel_FromEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)
	now := time.Now().UTC().Truncate(time.Second)
	holderID := uuid.New()

	tests := []struct {
		name    string
		holder  *mmodel.Holder
		wantErr bool
	}{
		{
			name: "minimal holder",
			holder: &mmodel.Holder{
				ID:        &holderID,
				Type:      testutils.Ptr("NATURAL_PERSON"),
				Name:      testutils.Ptr("John Doe"),
				Document:  testutils.Ptr("12345678901"),
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "holder with addresses",
			holder: &mmodel.Holder{
				ID:       &holderID,
				Type:     testutils.Ptr("NATURAL_PERSON"),
				Name:     testutils.Ptr("Jane Doe"),
				Document: testutils.Ptr("98765432100"),
				Addresses: &mmodel.Addresses{
					Primary: &mmodel.Address{
						Line1:   "123 Main St",
						Line2:   testutils.Ptr("Apt 4B"),
						ZipCode: "12345",
						City:    "New York",
						State:   "NY",
						Country: "US",
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "holder with contact",
			holder: &mmodel.Holder{
				ID:       &holderID,
				Type:     testutils.Ptr("NATURAL_PERSON"),
				Name:     testutils.Ptr("Bob Smith"),
				Document: testutils.Ptr("11122233344"),
				Contact: &mmodel.Contact{
					PrimaryEmail:   testutils.Ptr("bob@example.com"),
					SecondaryEmail: testutils.Ptr("bob.secondary@example.com"),
					MobilePhone:    testutils.Ptr("+1555123456"),
					OtherPhone:     testutils.Ptr("+1555654321"),
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "holder with natural person",
			holder: &mmodel.Holder{
				ID:       &holderID,
				Type:     testutils.Ptr("NATURAL_PERSON"),
				Name:     testutils.Ptr("Alice Johnson"),
				Document: testutils.Ptr("55566677788"),
				NaturalPerson: &mmodel.NaturalPerson{
					FavoriteName: testutils.Ptr("Alice"),
					SocialName:   testutils.Ptr("Alice J"),
					Gender:       testutils.Ptr("Female"),
					BirthDate:    testutils.Ptr("1990-05-15"),
					CivilStatus:  testutils.Ptr("Single"),
					Nationality:  testutils.Ptr("American"),
					MotherName:   testutils.Ptr("Mary Johnson"),
					FatherName:   testutils.Ptr("Robert Johnson"),
					Status:       testutils.Ptr("Active"),
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "holder with legal person",
			holder: &mmodel.Holder{
				ID:       &holderID,
				Type:     testutils.Ptr("LEGAL_PERSON"),
				Name:     testutils.Ptr("ACME Corp"),
				Document: testutils.Ptr("12345678000199"),
				LegalPerson: &mmodel.LegalPerson{
					TradeName:    testutils.Ptr("ACME"),
					Activity:     testutils.Ptr("Technology"),
					Type:         testutils.Ptr("LLC"),
					FoundingDate: testutils.Ptr("2020-01-15"),
					Size:         testutils.Ptr("Medium"),
					Status:       testutils.Ptr("Active"),
					Representative: &mmodel.Representative{
						Name:     testutils.Ptr("CEO Name"),
						Document: testutils.Ptr("99988877766"),
						Email:    testutils.Ptr("ceo@acme.com"),
						Role:     testutils.Ptr("CEO"),
					},
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "holder with metadata",
			holder: &mmodel.Holder{
				ID:       &holderID,
				Type:     testutils.Ptr("NATURAL_PERSON"),
				Name:     testutils.Ptr("Test User"),
				Document: testutils.Ptr("44455566677"),
				Metadata: map[string]any{
					"key1": "value1",
					"key2": 123,
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "holder with nil metadata initializes empty map",
			holder: &mmodel.Holder{
				ID:        &holderID,
				Type:      testutils.Ptr("NATURAL_PERSON"),
				Name:      testutils.Ptr("No Metadata User"),
				Document:  testutils.Ptr("77788899900"),
				Metadata:  nil,
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: false,
		},
		{
			name: "holder with all fields",
			holder: &mmodel.Holder{
				ID:         &holderID,
				ExternalID: testutils.Ptr("EXT-123"),
				Type:       testutils.Ptr("NATURAL_PERSON"),
				Name:       testutils.Ptr("Complete User"),
				Document:   testutils.Ptr("11111111111"),
				Addresses: &mmodel.Addresses{
					Primary: &mmodel.Address{
						Line1:   "Primary Address",
						ZipCode: "00000",
						City:    "City",
						State:   "ST",
						Country: "US",
					},
					Additional1: &mmodel.Address{
						Line1:   "Additional 1",
						ZipCode: "11111",
						City:    "City2",
						State:   "ST",
						Country: "US",
					},
					Additional2: &mmodel.Address{
						Line1:   "Additional 2",
						ZipCode: "22222",
						City:    "City3",
						State:   "ST",
						Country: "US",
					},
				},
				Contact: &mmodel.Contact{
					PrimaryEmail: testutils.Ptr("complete@example.com"),
					MobilePhone:  testutils.Ptr("+1234567890"),
				},
				NaturalPerson: &mmodel.NaturalPerson{
					FavoriteName: testutils.Ptr("Complete"),
					MotherName:   testutils.Ptr("Mother Name"),
					FatherName:   testutils.Ptr("Father Name"),
				},
				Metadata: map[string]any{
					"complete": true,
				},
				CreatedAt: now,
				UpdatedAt: now,
				DeletedAt: &now,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var model MongoDBModel
			err := model.FromEntity(tt.holder, crypto)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.holder.ID, model.ID)
			assert.Equal(t, tt.holder.ExternalID, model.ExternalID)
			assert.Equal(t, tt.holder.Type, model.Type)

			// Encrypted fields should not match original
			if tt.holder.Name != nil {
				assert.NotNil(t, model.Name)
				assert.NotEqual(t, *tt.holder.Name, *model.Name, "Name should be encrypted")
			}

			if tt.holder.Document != nil {
				assert.NotNil(t, model.Document)
				assert.NotEqual(t, *tt.holder.Document, *model.Document, "Document should be encrypted")
			}

			// Verify search hash is generated for document
			if tt.holder.Document != nil && *tt.holder.Document != "" {
				assert.NotEmpty(t, model.Search["document"], "Document hash should be generated")
			}

			// Verify metadata
			assert.NotNil(t, model.Metadata, "Metadata should never be nil")
		})
	}
}

func TestMongoDBModel_ToEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)
	now := time.Now().UTC().Truncate(time.Second)
	holderID := uuid.New()

	// First create a model from an entity, then convert back
	originalHolder := &mmodel.Holder{
		ID:         &holderID,
		ExternalID: testutils.Ptr("EXT-456"),
		Type:       testutils.Ptr("NATURAL_PERSON"),
		Name:       testutils.Ptr("Round Trip Test"),
		Document:   testutils.Ptr("33344455566"),
		Addresses: &mmodel.Addresses{
			Primary: &mmodel.Address{
				Line1:   "123 Test St",
				Line2:   testutils.Ptr("Suite 100"),
				ZipCode: "54321",
				City:    "TestCity",
				State:   "TS",
				Country: "TC",
			},
		},
		Contact: &mmodel.Contact{
			PrimaryEmail:   testutils.Ptr("roundtrip@test.com"),
			SecondaryEmail: testutils.Ptr("secondary@test.com"),
			MobilePhone:    testutils.Ptr("+9876543210"),
			OtherPhone:     testutils.Ptr("+1234567890"),
		},
		NaturalPerson: &mmodel.NaturalPerson{
			FavoriteName: testutils.Ptr("RT"),
			SocialName:   testutils.Ptr("RoundTrip"),
			Gender:       testutils.Ptr("Other"),
			BirthDate:    testutils.Ptr("1985-12-25"),
			CivilStatus:  testutils.Ptr("Married"),
			Nationality:  testutils.Ptr("TestNation"),
			MotherName:   testutils.Ptr("Test Mother"),
			FatherName:   testutils.Ptr("Test Father"),
			Status:       testutils.Ptr("Active"),
		},
		Metadata: map[string]any{
			"testKey": "testValue",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	var model MongoDBModel
	err := model.FromEntity(originalHolder, crypto)
	require.NoError(t, err)

	// Now convert back to entity
	resultHolder, err := model.ToEntity(crypto)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, originalHolder.ID, resultHolder.ID)
	assert.Equal(t, originalHolder.ExternalID, resultHolder.ExternalID)
	assert.Equal(t, originalHolder.Type, resultHolder.Type)
	assert.Equal(t, *originalHolder.Name, *resultHolder.Name)
	assert.Equal(t, *originalHolder.Document, *resultHolder.Document)
	assert.Equal(t, originalHolder.Metadata, resultHolder.Metadata)

	// Verify addresses
	require.NotNil(t, resultHolder.Addresses)
	require.NotNil(t, resultHolder.Addresses.Primary)
	assert.Equal(t, originalHolder.Addresses.Primary.Line1, resultHolder.Addresses.Primary.Line1)
	assert.Equal(t, originalHolder.Addresses.Primary.City, resultHolder.Addresses.Primary.City)

	// Verify contact (decrypted)
	require.NotNil(t, resultHolder.Contact)
	assert.Equal(t, *originalHolder.Contact.PrimaryEmail, *resultHolder.Contact.PrimaryEmail)
	assert.Equal(t, *originalHolder.Contact.MobilePhone, *resultHolder.Contact.MobilePhone)

	// Verify natural person (decrypted)
	require.NotNil(t, resultHolder.NaturalPerson)
	assert.Equal(t, originalHolder.NaturalPerson.FavoriteName, resultHolder.NaturalPerson.FavoriteName)
	assert.Equal(t, *originalHolder.NaturalPerson.MotherName, *resultHolder.NaturalPerson.MotherName)
	assert.Equal(t, *originalHolder.NaturalPerson.FatherName, *resultHolder.NaturalPerson.FatherName)
}

func TestMongoDBModel_ToEntity_LegalPerson(t *testing.T) {
	crypto := testutils.SetupCrypto(t)
	now := time.Now().UTC().Truncate(time.Second)
	holderID := uuid.New()

	originalHolder := &mmodel.Holder{
		ID:       &holderID,
		Type:     testutils.Ptr("LEGAL_PERSON"),
		Name:     testutils.Ptr("Legal Entity Corp"),
		Document: testutils.Ptr("12345678000199"),
		LegalPerson: &mmodel.LegalPerson{
			TradeName:    testutils.Ptr("Legal Entity"),
			Activity:     testutils.Ptr("Consulting"),
			Type:         testutils.Ptr("Corporation"),
			FoundingDate: testutils.Ptr("2015-06-01"),
			Size:         testutils.Ptr("Large"),
			Status:       testutils.Ptr("Active"),
			Representative: &mmodel.Representative{
				Name:     testutils.Ptr("Legal Rep"),
				Document: testutils.Ptr("11122233344"),
				Email:    testutils.Ptr("rep@legalentity.com"),
				Role:     testutils.Ptr("Director"),
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	var model MongoDBModel
	err := model.FromEntity(originalHolder, crypto)
	require.NoError(t, err)

	resultHolder, err := model.ToEntity(crypto)
	require.NoError(t, err)

	require.NotNil(t, resultHolder.LegalPerson)
	assert.Equal(t, originalHolder.LegalPerson.TradeName, resultHolder.LegalPerson.TradeName)
	assert.Equal(t, originalHolder.LegalPerson.Activity, resultHolder.LegalPerson.Activity)
	assert.Equal(t, originalHolder.LegalPerson.FoundingDate, resultHolder.LegalPerson.FoundingDate)

	require.NotNil(t, resultHolder.LegalPerson.Representative)
	assert.Equal(t, *originalHolder.LegalPerson.Representative.Name, *resultHolder.LegalPerson.Representative.Name)
	assert.Equal(t, *originalHolder.LegalPerson.Representative.Document, *resultHolder.LegalPerson.Representative.Document)
	assert.Equal(t, *originalHolder.LegalPerson.Representative.Email, *resultHolder.LegalPerson.Representative.Email)
	assert.Equal(t, originalHolder.LegalPerson.Representative.Role, resultHolder.LegalPerson.Representative.Role)
}

func TestMapAddressFromEntity(t *testing.T) {
	tests := []struct {
		name    string
		address *mmodel.Address
		wantNil bool
	}{
		{
			name:    "nil address returns nil",
			address: nil,
			wantNil: true,
		},
		{
			name: "complete address",
			address: &mmodel.Address{
				Line1:   "123 Main St",
				Line2:   testutils.Ptr("Apt 1"),
				ZipCode: "12345",
				City:    "TestCity",
				State:   "TS",
				Country: "TC",
			},
			wantNil: false,
		},
		{
			name: "minimal address",
			address: &mmodel.Address{
				Line1:   "Minimal",
				ZipCode: "00000",
				City:    "City",
				State:   "ST",
				Country: "US",
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapAddressFromEntity(tt.address)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.address.Line1, *result.Line1)
			assert.Equal(t, tt.address.ZipCode, *result.ZipCode)
			assert.Equal(t, tt.address.City, *result.City)
			assert.Equal(t, tt.address.State, *result.State)
			assert.Equal(t, tt.address.Country, *result.Country)
		})
	}
}

func TestMapAddressToEntity(t *testing.T) {
	tests := []struct {
		name    string
		model   *AddressMongoDBModel
		wantNil bool
	}{
		{
			name:    "nil model returns nil",
			model:   nil,
			wantNil: true,
		},
		{
			name: "complete model",
			model: &AddressMongoDBModel{
				Line1:   testutils.Ptr("456 Test Ave"),
				Line2:   testutils.Ptr("Floor 2"),
				ZipCode: testutils.Ptr("67890"),
				City:    testutils.Ptr("ModelCity"),
				State:   testutils.Ptr("MC"),
				Country: testutils.Ptr("MD"),
			},
			wantNil: false,
		},
		{
			name: "model with nil fields",
			model: &AddressMongoDBModel{
				Line1:   nil,
				Line2:   nil,
				ZipCode: nil,
				City:    nil,
				State:   nil,
				Country: nil,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapAddressToEntity(tt.model)

			if tt.wantNil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			if tt.model.Line1 != nil {
				assert.Equal(t, *tt.model.Line1, result.Line1)
			} else {
				assert.Empty(t, result.Line1)
			}
		})
	}
}

func TestMapAddressesToEntity(t *testing.T) {
	model := &AddressesMongoDBModel{
		Primary: &AddressMongoDBModel{
			Line1:   testutils.Ptr("Primary St"),
			ZipCode: testutils.Ptr("11111"),
			City:    testutils.Ptr("PrimaryCity"),
			State:   testutils.Ptr("PC"),
			Country: testutils.Ptr("PR"),
		},
		Additional1: &AddressMongoDBModel{
			Line1:   testutils.Ptr("Additional1 St"),
			ZipCode: testutils.Ptr("22222"),
			City:    testutils.Ptr("Add1City"),
			State:   testutils.Ptr("A1"),
			Country: testutils.Ptr("AD"),
		},
		Additional2: nil,
	}

	result := mapAddressesToEntity(model)

	require.NotNil(t, result)
	require.NotNil(t, result.Primary)
	assert.Equal(t, "Primary St", result.Primary.Line1)

	require.NotNil(t, result.Additional1)
	assert.Equal(t, "Additional1 St", result.Additional1.Line1)

	assert.Nil(t, result.Additional2)
}

func TestMapContactFromEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)

	contact := &mmodel.Contact{
		PrimaryEmail:   testutils.Ptr("primary@test.com"),
		SecondaryEmail: testutils.Ptr("secondary@test.com"),
		MobilePhone:    testutils.Ptr("+1234567890"),
		OtherPhone:     testutils.Ptr("+0987654321"),
	}

	result, err := mapContactFromEntity(crypto, contact)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Fields should be encrypted (not equal to originals)
	assert.NotEqual(t, *contact.PrimaryEmail, *result.PrimaryEmail)
	assert.NotEqual(t, *contact.SecondaryEmail, *result.SecondaryEmail)
	assert.NotEqual(t, *contact.MobilePhone, *result.MobilePhone)
	assert.NotEqual(t, *contact.OtherPhone, *result.OtherPhone)
}

func TestMapContactToEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)

	// First encrypt contact data
	originalContact := &mmodel.Contact{
		PrimaryEmail:   testutils.Ptr("decrypt@test.com"),
		SecondaryEmail: testutils.Ptr("decrypt2@test.com"),
		MobilePhone:    testutils.Ptr("+1111111111"),
		OtherPhone:     testutils.Ptr("+2222222222"),
	}

	encryptedModel, err := mapContactFromEntity(crypto, originalContact)
	require.NoError(t, err)

	// Now decrypt back
	result, err := mapContactToEntity(crypto, encryptedModel)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, *originalContact.PrimaryEmail, *result.PrimaryEmail)
	assert.Equal(t, *originalContact.SecondaryEmail, *result.SecondaryEmail)
	assert.Equal(t, *originalContact.MobilePhone, *result.MobilePhone)
	assert.Equal(t, *originalContact.OtherPhone, *result.OtherPhone)
}

func TestMapNaturalPersonFromEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)

	np := &mmodel.NaturalPerson{
		FavoriteName: testutils.Ptr("Favorite"),
		SocialName:   testutils.Ptr("Social"),
		Gender:       testutils.Ptr("Male"),
		BirthDate:    testutils.Ptr("1990-01-01"),
		CivilStatus:  testutils.Ptr("Single"),
		Nationality:  testutils.Ptr("Brazilian"),
		MotherName:   testutils.Ptr("Mother"),
		FatherName:   testutils.Ptr("Father"),
		Status:       testutils.Ptr("Active"),
	}

	result, err := mapNaturalPersonFromEntity(crypto, np)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Non-encrypted fields
	assert.Equal(t, np.FavoriteName, result.FavoriteName)
	assert.Equal(t, np.SocialName, result.SocialName)
	assert.Equal(t, np.Gender, result.Gender)
	assert.Equal(t, np.BirthDate, result.BirthDate)
	assert.Equal(t, np.CivilStatus, result.CivilStatus)
	assert.Equal(t, np.Nationality, result.Nationality)
	assert.Equal(t, np.Status, result.Status)

	// Encrypted fields should differ
	assert.NotEqual(t, *np.MotherName, *result.MotherName)
	assert.NotEqual(t, *np.FatherName, *result.FatherName)
}

func TestMapNaturalPersonToEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)

	originalNP := &mmodel.NaturalPerson{
		FavoriteName: testutils.Ptr("TestFav"),
		SocialName:   testutils.Ptr("TestSocial"),
		MotherName:   testutils.Ptr("TestMother"),
		FatherName:   testutils.Ptr("TestFather"),
	}

	encryptedModel, err := mapNaturalPersonFromEntity(crypto, originalNP)
	require.NoError(t, err)

	result, err := mapNaturalPersonToEntity(crypto, encryptedModel)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, *originalNP.MotherName, *result.MotherName)
	assert.Equal(t, *originalNP.FatherName, *result.FatherName)
}

func TestMapLegalPersonFromEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)

	lp := &mmodel.LegalPerson{
		TradeName:    testutils.Ptr("Trade"),
		Activity:     testutils.Ptr("Activity"),
		Type:         testutils.Ptr("LLC"),
		FoundingDate: testutils.Ptr("2020-06-15"),
		Size:         testutils.Ptr("Small"),
		Status:       testutils.Ptr("Active"),
		Representative: &mmodel.Representative{
			Name:     testutils.Ptr("Rep Name"),
			Document: testutils.Ptr("12345678900"),
			Email:    testutils.Ptr("rep@company.com"),
			Role:     testutils.Ptr("CEO"),
		},
	}

	result, err := mapLegalPersonFromEntity(crypto, lp)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Non-encrypted fields
	assert.Equal(t, lp.TradeName, result.TradeName)
	assert.Equal(t, lp.Activity, result.Activity)
	assert.Equal(t, lp.Type, result.Type)
	assert.Equal(t, lp.Size, result.Size)
	assert.Equal(t, lp.Status, result.Status)

	// Founding date should be parsed
	require.NotNil(t, result.FoundingDate)
	assert.Equal(t, 2020, result.FoundingDate.Year())
	assert.Equal(t, time.June, result.FoundingDate.Month())
	assert.Equal(t, 15, result.FoundingDate.Day())

	// Representative encrypted fields
	require.NotNil(t, result.Representative)
	assert.NotEqual(t, *lp.Representative.Name, *result.Representative.Name)
	assert.NotEqual(t, *lp.Representative.Document, *result.Representative.Document)
	assert.NotEqual(t, *lp.Representative.Email, *result.Representative.Email)
	assert.Equal(t, lp.Representative.Role, result.Representative.Role) // Role is not encrypted
}

func TestMapLegalPersonFromEntity_InvalidFoundingDate(t *testing.T) {
	crypto := testutils.SetupCrypto(t)

	lp := &mmodel.LegalPerson{
		TradeName:    testutils.Ptr("Trade"),
		FoundingDate: testutils.Ptr("invalid-date"),
	}

	_, err := mapLegalPersonFromEntity(crypto, lp)
	require.Error(t, err)
}

func TestMapLegalPersonFromEntity_NilFoundingDate(t *testing.T) {
	crypto := testutils.SetupCrypto(t)

	lp := &mmodel.LegalPerson{
		TradeName:    testutils.Ptr("Trade"),
		FoundingDate: nil,
	}

	result, err := mapLegalPersonFromEntity(crypto, lp)
	require.NoError(t, err)
	assert.Nil(t, result.FoundingDate)
}

func TestMapLegalPersonToEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)

	originalLP := &mmodel.LegalPerson{
		TradeName:    testutils.Ptr("Original Trade"),
		Activity:     testutils.Ptr("Original Activity"),
		Type:         testutils.Ptr("Corporation"),
		FoundingDate: testutils.Ptr("2018-03-20"),
		Size:         testutils.Ptr("Large"),
		Status:       testutils.Ptr("Active"),
		Representative: &mmodel.Representative{
			Name:     testutils.Ptr("Original Rep"),
			Document: testutils.Ptr("99988877766"),
			Email:    testutils.Ptr("original@company.com"),
			Role:     testutils.Ptr("CFO"),
		},
	}

	encryptedModel, err := mapLegalPersonFromEntity(crypto, originalLP)
	require.NoError(t, err)

	result, err := mapLegalPersonToEntity(crypto, encryptedModel)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, originalLP.TradeName, result.TradeName)
	assert.Equal(t, originalLP.FoundingDate, result.FoundingDate)

	require.NotNil(t, result.Representative)
	assert.Equal(t, *originalLP.Representative.Name, *result.Representative.Name)
	assert.Equal(t, *originalLP.Representative.Document, *result.Representative.Document)
	assert.Equal(t, *originalLP.Representative.Email, *result.Representative.Email)
}

func TestMapRepresentativeToEntity(t *testing.T) {
	crypto := testutils.SetupCrypto(t)

	// Encrypt representative data
	name, err := crypto.Encrypt(testutils.Ptr("Rep Name"))
	require.NoError(t, err)
	doc, err := crypto.Encrypt(testutils.Ptr("12345678900"))
	require.NoError(t, err)
	email, err := crypto.Encrypt(testutils.Ptr("rep@test.com"))
	require.NoError(t, err)

	model := &RepresentativeMongoDBModel{
		Name:     name,
		Document: doc,
		Email:    email,
		Role:     testutils.Ptr("Director"),
	}

	result, err := mapRepresentativeToEntity(crypto, model)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "Rep Name", *result.Name)
	assert.Equal(t, "12345678900", *result.Document)
	assert.Equal(t, "rep@test.com", *result.Email)
	assert.Equal(t, "Director", *result.Role)
}
