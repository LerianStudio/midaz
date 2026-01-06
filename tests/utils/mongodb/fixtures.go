//go:build integration

package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	testutils "github.com/LerianStudio/midaz/v3/tests/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// MetadataFixture represents test metadata for insertion.
type MetadataFixture struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	EntityID   string             `bson:"entity_id"`
	EntityName string             `bson:"entity_name"`
	Data       map[string]any     `bson:"metadata"`
	CreatedAt  time.Time          `bson:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at"`
}

// InsertMetadata inserts a metadata fixture into the specified collection.
func InsertMetadata(t *testing.T, db *mongo.Database, collection string, fixture MetadataFixture) primitive.ObjectID {
	t.Helper()

	if fixture.ID.IsZero() {
		fixture.ID = primitive.NewObjectID()
	}

	if fixture.CreatedAt.IsZero() {
		fixture.CreatedAt = time.Now()
	}

	if fixture.UpdatedAt.IsZero() {
		fixture.UpdatedAt = time.Now()
	}

	coll := db.Collection(collection)
	_, err := coll.InsertOne(context.Background(), fixture)
	require.NoError(t, err, "failed to insert metadata fixture")

	return fixture.ID
}

// InsertManyMetadata inserts multiple metadata fixtures into the specified collection.
func InsertManyMetadata(t *testing.T, db *mongo.Database, collection string, fixtures []MetadataFixture) []primitive.ObjectID {
	t.Helper()

	ids := make([]primitive.ObjectID, len(fixtures))
	for i, f := range fixtures {
		ids[i] = InsertMetadata(t, db, collection, f)
	}

	return ids
}

// CountDocuments counts documents in a collection with optional filter.
func CountDocuments(t *testing.T, db *mongo.Database, collection string, filter bson.M) int64 {
	t.Helper()

	if filter == nil {
		filter = bson.M{}
	}

	count, err := db.Collection(collection).CountDocuments(context.Background(), filter)
	require.NoError(t, err, "failed to count documents")

	return count
}

// ClearCollection removes all documents from a collection.
func ClearCollection(t *testing.T, db *mongo.Database, collection string) {
	t.Helper()

	_, err := db.Collection(collection).DeleteMany(context.Background(), bson.M{})
	require.NoError(t, err, "failed to clear collection")
}

// ============================================================================
// Alias Fixtures
// ============================================================================

// AliasParams holds parameters for creating a test alias.
type AliasParams struct {
	Document   string
	Type       string
	LedgerID   string
	AccountID  string
	Metadata   map[string]any
	DeletedAt  *time.Time
	ClosingDate *time.Time
}

// DefaultAliasParams returns default parameters for creating a test alias.
func DefaultAliasParams() AliasParams {
	return AliasParams{
		Document: "12345678901",
		Type:     "NATURAL_PERSON",
		LedgerID: "ledger-" + uuid.New().String()[:8],
		Metadata: map[string]any{"test": true},
	}
}

// CreateTestAlias builds a test alias with the given holder ID and params.
func CreateTestAlias(t *testing.T, holderID uuid.UUID, params AliasParams) *mmodel.Alias {
	t.Helper()

	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	return &mmodel.Alias{
		ID:          &id,
		Document:    testutils.Ptr(params.Document),
		Type:        testutils.Ptr(params.Type),
		LedgerID:    testutils.Ptr(params.LedgerID),
		AccountID:   testutils.Ptr(params.AccountID),
		HolderID:    &holderID,
		Metadata:    params.Metadata,
		ClosingDate: params.ClosingDate,
		CreatedAt:   now,
		UpdatedAt:   now,
		DeletedAt:   params.DeletedAt,
	}
}

// CreateTestAliasSimple builds a test alias with default values.
func CreateTestAliasSimple(t *testing.T, holderID uuid.UUID, accountID, document string) *mmodel.Alias {
	t.Helper()

	params := DefaultAliasParams()
	params.AccountID = accountID
	params.Document = document

	return CreateTestAlias(t, holderID, params)
}

// BankingDetailsParams holds parameters for banking details.
type BankingDetailsParams struct {
	Branch      string
	Account     string
	Type        string
	OpeningDate string
	IBAN        string
	CountryCode string
	BankID      string
}

// DefaultBankingDetailsParams returns default banking details params.
func DefaultBankingDetailsParams() BankingDetailsParams {
	return BankingDetailsParams{
		Branch:      "0001",
		Account:     "123456",
		Type:        "CACC",
		OpeningDate: "2025-01-01",
		IBAN:        "BR1234567890123456789012345",
		CountryCode: "BR",
		BankID:      "001",
	}
}

// CreateBankingDetails builds banking details from params.
func CreateBankingDetails(params BankingDetailsParams) *mmodel.BankingDetails {
	return &mmodel.BankingDetails{
		Branch:      testutils.Ptr(params.Branch),
		Account:     testutils.Ptr(params.Account),
		Type:        testutils.Ptr(params.Type),
		OpeningDate: testutils.Ptr(params.OpeningDate),
		IBAN:        testutils.Ptr(params.IBAN),
		CountryCode: testutils.Ptr(params.CountryCode),
		BankID:      testutils.Ptr(params.BankID),
	}
}

// CreateTestAliasWithBanking builds a test alias with banking details.
func CreateTestAliasWithBanking(t *testing.T, holderID uuid.UUID, accountID, document string) *mmodel.Alias {
	t.Helper()

	alias := CreateTestAliasSimple(t, holderID, accountID, document)
	alias.BankingDetails = CreateBankingDetails(DefaultBankingDetailsParams())

	return alias
}

// ============================================================================
// Holder Fixtures
// ============================================================================

// HolderParams holds parameters for creating a test holder.
type HolderParams struct {
	Name       string
	Document   string
	Type       string
	ExternalID *string
	Metadata   map[string]any
	DeletedAt  *time.Time
}

// DefaultHolderParams returns default parameters for creating a test holder.
func DefaultHolderParams() HolderParams {
	return HolderParams{
		Name:     "Test Holder",
		Document: "12345678901",
		Type:     "NATURAL_PERSON",
		Metadata: map[string]any{"test": true},
	}
}

// CreateTestHolder builds a test holder with the given params.
func CreateTestHolder(t *testing.T, params HolderParams) *mmodel.Holder {
	t.Helper()

	id := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	return &mmodel.Holder{
		ID:         &id,
		Type:       testutils.Ptr(params.Type),
		Name:       testutils.Ptr(params.Name),
		Document:   testutils.Ptr(params.Document),
		ExternalID: params.ExternalID,
		Metadata:   params.Metadata,
		CreatedAt:  now,
		UpdatedAt:  now,
		DeletedAt:  params.DeletedAt,
	}
}

// CreateTestHolderSimple builds a test holder with name and document.
func CreateTestHolderSimple(t *testing.T, name, document string) *mmodel.Holder {
	t.Helper()

	params := DefaultHolderParams()
	params.Name = name
	params.Document = document

	return CreateTestHolder(t, params)
}

// CreateTestHolderWithExternalID builds a test holder with external ID.
func CreateTestHolderWithExternalID(t *testing.T, name, document, externalID string) *mmodel.Holder {
	t.Helper()

	holder := CreateTestHolderSimple(t, name, document)
	holder.ExternalID = testutils.Ptr(externalID)

	return holder
}

// CreateTestHolderWithContact builds a test holder with contact info.
func CreateTestHolderWithContact(t *testing.T, name, document string) *mmodel.Holder {
	t.Helper()

	holder := CreateTestHolderSimple(t, name, document)
	holder.Contact = &mmodel.Contact{
		PrimaryEmail:   testutils.Ptr("primary@example.com"),
		SecondaryEmail: testutils.Ptr("secondary@example.com"),
		MobilePhone:    testutils.Ptr("+1234567890"),
		OtherPhone:     testutils.Ptr("+0987654321"),
	}

	return holder
}

// CreateTestHolderWithNaturalPerson builds a test holder as natural person.
func CreateTestHolderWithNaturalPerson(t *testing.T, name, document string) *mmodel.Holder {
	t.Helper()

	holder := CreateTestHolderSimple(t, name, document)
	holder.NaturalPerson = &mmodel.NaturalPerson{
		FavoriteName: testutils.Ptr("Favorite"),
		SocialName:   testutils.Ptr("Social"),
		Gender:       testutils.Ptr("Male"),
		BirthDate:    testutils.Ptr("1990-05-15"),
		CivilStatus:  testutils.Ptr("Single"),
		Nationality:  testutils.Ptr("Brazilian"),
		MotherName:   testutils.Ptr("Mother Name"),
		FatherName:   testutils.Ptr("Father Name"),
		Status:       testutils.Ptr("Active"),
	}

	return holder
}

// CreateTestHolderWithLegalPerson builds a test holder as legal person.
func CreateTestHolderWithLegalPerson(t *testing.T, name, document string) *mmodel.Holder {
	t.Helper()

	holder := CreateTestHolderSimple(t, name, document)
	holder.Type = testutils.Ptr("LEGAL_PERSON")
	holder.LegalPerson = &mmodel.LegalPerson{
		TradeName:    testutils.Ptr("Trade Name"),
		Activity:     testutils.Ptr("Technology"),
		Type:         testutils.Ptr("LLC"),
		FoundingDate: testutils.Ptr("2020-01-15"),
		Size:         testutils.Ptr("Medium"),
		Status:       testutils.Ptr("Active"),
		Representative: &mmodel.Representative{
			Name:     testutils.Ptr("Representative Name"),
			Document: testutils.Ptr("99988877766"),
			Email:    testutils.Ptr("rep@company.com"),
			Role:     testutils.Ptr("CEO"),
		},
	}

	return holder
}

// CreateTestHolderWithAddresses builds a test holder with addresses.
func CreateTestHolderWithAddresses(t *testing.T, name, document string) *mmodel.Holder {
	t.Helper()

	holder := CreateTestHolderSimple(t, name, document)
	holder.Addresses = &mmodel.Addresses{
		Primary: &mmodel.Address{
			Line1:   "123 Main St",
			Line2:   testutils.Ptr("Apt 4B"),
			ZipCode: "12345",
			City:    "New York",
			State:   "NY",
			Country: "US",
		},
		Additional1: &mmodel.Address{
			Line1:   "456 Secondary Ave",
			ZipCode: "67890",
			City:    "Los Angeles",
			State:   "CA",
			Country: "US",
		},
	}

	return holder
}

// CreateCompleteTestHolder builds a test holder with all fields populated.
func CreateCompleteTestHolder(t *testing.T, name, document string) *mmodel.Holder {
	t.Helper()

	holder := CreateTestHolderWithNaturalPerson(t, name, document)
	holder.ExternalID = testutils.Ptr("EXT-" + uuid.New().String()[:8])
	holder.Addresses = &mmodel.Addresses{
		Primary: &mmodel.Address{
			Line1:   "Complete Address",
			ZipCode: "00000",
			City:    "Complete City",
			State:   "CS",
			Country: "CC",
		},
	}
	holder.Contact = &mmodel.Contact{
		PrimaryEmail: testutils.Ptr("complete@example.com"),
		MobilePhone:  testutils.Ptr("+1111111111"),
	}

	return holder
}

