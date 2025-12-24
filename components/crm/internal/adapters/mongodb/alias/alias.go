package alias

import (
	"time"

	libCrypto "github.com/LerianStudio/lib-commons/v2/commons/crypto"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

type MongoDBModel struct {
	ID                  *uuid.UUID           `bson:"_id,omitempty"`
	Document            *string              `bson:"document,omitempty"`
	Type                *string              `bson:"type,omitempty"`
	LedgerID            *string              `bson:"ledger_id,omitempty"`
	AccountID           *string              `bson:"account_id,omitempty"`
	HolderID            *uuid.UUID           `bson:"holder_id,omitempty"`
	Metadata            map[string]any       `bson:"metadata"`
	Search              map[string]string    `bson:"search,omitempty"`
	BankingDetails      *BankingMongoDBModel `bson:"banking_details,omitempty"`
	ParticipantDocument *string              `bson:"participant_document,omitempty"`
	ClosingDate         *time.Time           `bson:"closing_date,omitempty"`
	CreatedAt           *time.Time           `bson:"created_at,omitempty"`
	UpdatedAt           *time.Time           `bson:"updated_at"`
	DeletedAt           *time.Time           `bson:"deleted_at"`
}

type BankingMongoDBModel struct {
	Branch      *string `bson:"branch,omitempty"`
	Account     *string `bson:"account,omitempty"`
	Type        *string `bson:"type,omitempty"`
	OpeningDate *string `bson:"opening_date,omitempty"`
	IBAN        *string `bson:"iban,omitempty"`
	CountryCode *string `bson:"country_code,omitempty"`
	BankID      *string `bson:"bank_id,omitempty"`
}

// FromEntity maps an account entity to a MongoDB Alias model
func (amm *MongoDBModel) FromEntity(a *mmodel.Alias, ds *libCrypto.Crypto) error {
	document, err := ds.Encrypt(a.Document)
	if err != nil {
		return pkg.ValidateInternalError(err, "Alias")
	}

	participantDocument, err := ds.Encrypt(a.ParticipantDocument)
	if err != nil {
		return pkg.ValidateInternalError(err, "Alias")
	}

	*amm = MongoDBModel{
		ID:                  a.ID,
		Document:            document,
		Type:                a.Type,
		LedgerID:            a.LedgerID,
		AccountID:           a.AccountID,
		HolderID:            a.HolderID,
		ParticipantDocument: participantDocument,
		ClosingDate:         a.ClosingDate,
		CreatedAt:           &a.CreatedAt,
		UpdatedAt:           &a.UpdatedAt,
		DeletedAt:           a.DeletedAt,
	}

	amm.Search = make(map[string]string)
	amm.addDocumentToSearch(a.Document, ds)

	if err := amm.setBankingDetails(a.BankingDetails, ds); err != nil {
		return err
	}

	if a.Metadata == nil {
		amm.Metadata = make(map[string]any)
	} else {
		amm.Metadata = a.Metadata
	}

	return nil
}

func (amm *MongoDBModel) addDocumentToSearch(document *string, ds *libCrypto.Crypto) {
	if document != nil && *document != "" {
		amm.Search["document"] = ds.GenerateHash(document)
	}
}

func (amm *MongoDBModel) setBankingDetails(details *mmodel.BankingDetails, ds *libCrypto.Crypto) error {
	if details == nil {
		return nil
	}

	account, err := ds.Encrypt(details.Account)
	if err != nil {
		return pkg.ValidateInternalError(err, "Alias")
	}

	iban, err := ds.Encrypt(details.IBAN)
	if err != nil {
		return pkg.ValidateInternalError(err, "Alias")
	}

	amm.BankingDetails = &BankingMongoDBModel{
		Branch:      details.Branch,
		Account:     account,
		Type:        details.Type,
		OpeningDate: details.OpeningDate,
		CountryCode: details.CountryCode,
		BankID:      details.BankID,
		IBAN:        iban,
	}

	amm.addBankingDetailsToSearch(details, ds)

	return nil
}

func (amm *MongoDBModel) addBankingDetailsToSearch(details *mmodel.BankingDetails, ds *libCrypto.Crypto) {
	if details.Account != nil && *details.Account != "" {
		amm.Search["banking_details_account"] = ds.GenerateHash(details.Account)
	}

	if details.IBAN != nil && *details.IBAN != "" {
		amm.Search["banking_details_iban"] = ds.GenerateHash(details.IBAN)
	}
}

// ToEntity maps a MongoDB model to an Alias entity
func (amm *MongoDBModel) ToEntity(ds *libCrypto.Crypto) (*mmodel.Alias, error) {
	document, err := ds.Decrypt(amm.Document)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Alias")
	}

	participantDocument, err := ds.Decrypt(amm.ParticipantDocument)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Alias")
	}

	account := &mmodel.Alias{
		ID:                  amm.ID,
		Document:            document,
		Type:                amm.Type,
		LedgerID:            amm.LedgerID,
		AccountID:           amm.AccountID,
		HolderID:            amm.HolderID,
		Metadata:            amm.Metadata,
		ParticipantDocument: participantDocument,
		ClosingDate:         amm.ClosingDate,
		CreatedAt:           *amm.CreatedAt,
		UpdatedAt:           *amm.UpdatedAt,
		DeletedAt:           amm.DeletedAt,
	}

	if amm.BankingDetails != nil {
		accountNumber, err := ds.Decrypt(amm.BankingDetails.Account)
		if err != nil {
			return nil, pkg.ValidateInternalError(err, "Alias")
		}

		iban, err := ds.Decrypt(amm.BankingDetails.IBAN)
		if err != nil {
			return nil, pkg.ValidateInternalError(err, "Alias")
		}

		account.BankingDetails = &mmodel.BankingDetails{
			Branch:      amm.BankingDetails.Branch,
			Account:     accountNumber,
			Type:        amm.BankingDetails.Type,
			OpeningDate: amm.BankingDetails.OpeningDate,
			IBAN:        iban,
			CountryCode: amm.BankingDetails.CountryCode,
			BankID:      amm.BankingDetails.BankID,
		}
	}

	return account, nil
}
