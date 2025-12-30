package alias

import (
	"time"

	libCrypto "github.com/LerianStudio/lib-commons/v2/commons/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

type MongoDBModel struct {
	ID               *uuid.UUID                  `bson:"_id,omitempty"`
	Document         *string                     `bson:"document,omitempty"`
	Type             *string                     `bson:"type,omitempty"`
	LedgerID         *string                     `bson:"ledger_id,omitempty"`
	AccountID        *string                     `bson:"account_id,omitempty"`
	HolderID         *uuid.UUID                  `bson:"holder_id,omitempty"`
	Metadata         map[string]any              `bson:"metadata"`
	Search           *SearchMongoDB              `bson:"search,omitempty"`
	BankingDetails   *BankingMongoDBModel        `bson:"banking_details,omitempty"`
	RegulatoryFields *RegulatoryFieldsMongoDBModel `bson:"regulatory_fields,omitempty"`
	RelatedParties   []*RelatedPartyMongoDBModel `bson:"related_parties,omitempty"`
	ClosingDate      *time.Time                  `bson:"closing_date,omitempty"`
	CreatedAt        *time.Time                  `bson:"created_at,omitempty"`
	UpdatedAt        *time.Time                  `bson:"updated_at"`
	DeletedAt        *time.Time                  `bson:"deleted_at"`
}

type SearchMongoDB struct {
	Document              *string  `bson:"document,omitempty"`
	BankingDetailsAccount *string  `bson:"banking_details_account,omitempty"`
	BankingDetailsIBAN    *string  `bson:"banking_details_iban,omitempty"`
	RelatedPartyDocuments []string `bson:"related_party_documents,omitempty"`
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

type RegulatoryFieldsMongoDBModel struct {
	ParticipantDocument *string `bson:"participant_document,omitempty"`
}

type RelatedPartyMongoDBModel struct {
	ID        *uuid.UUID `bson:"_id"`
	Document  *string    `bson:"document"`
	Name      string     `bson:"name"`
	Role      string     `bson:"role"`
	StartDate time.Time  `bson:"start_date"`
	EndDate   *time.Time `bson:"end_date,omitempty"`
}

// FromEntity maps an account entity to a MongoDB Alias model
func (amm *MongoDBModel) FromEntity(a *mmodel.Alias, ds *libCrypto.Crypto) error {
	document, err := ds.Encrypt(a.Document)
	if err != nil {
		return err
	}

	*amm = MongoDBModel{
		ID:          a.ID,
		Document:    document,
		Type:        a.Type,
		LedgerID:    a.LedgerID,
		AccountID:   a.AccountID,
		HolderID:    a.HolderID,
		ClosingDate: a.ClosingDate,
		CreatedAt:   &a.CreatedAt,
		UpdatedAt:   &a.UpdatedAt,
		DeletedAt:   a.DeletedAt,
	}

	amm.Search = &SearchMongoDB{}

	if a.Document != nil && *a.Document != "" {
		hash := ds.GenerateHash(a.Document)
		amm.Search.Document = &hash
	}

	if a.BankingDetails != nil {
		account, err := ds.Encrypt(a.BankingDetails.Account)
		if err != nil {
			return err
		}

		iban, err := ds.Encrypt(a.BankingDetails.IBAN)
		if err != nil {
			return err
		}

		amm.BankingDetails = &BankingMongoDBModel{
			Branch:      a.BankingDetails.Branch,
			Account:     account,
			Type:        a.BankingDetails.Type,
			OpeningDate: a.BankingDetails.OpeningDate,
			CountryCode: a.BankingDetails.CountryCode,
			BankID:      a.BankingDetails.BankID,
			IBAN:        iban,
		}

		if a.BankingDetails.Account != nil && *a.BankingDetails.Account != "" {
			hash := ds.GenerateHash(a.BankingDetails.Account)
			amm.Search.BankingDetailsAccount = &hash
		}

		if a.BankingDetails.IBAN != nil && *a.BankingDetails.IBAN != "" {
			hash := ds.GenerateHash(a.BankingDetails.IBAN)
			amm.Search.BankingDetailsIBAN = &hash
		}
	}

	if a.RegulatoryFields != nil {
		participantDocument, err := ds.Encrypt(a.RegulatoryFields.ParticipantDocument)
		if err != nil {
			return err
		}

		amm.RegulatoryFields = &RegulatoryFieldsMongoDBModel{
			ParticipantDocument: participantDocument,
		}
	}

	if len(a.RelatedParties) > 0 {
		amm.RelatedParties = make([]*RelatedPartyMongoDBModel, len(a.RelatedParties))
		amm.Search.RelatedPartyDocuments = make([]string, 0, len(a.RelatedParties))

		for i, rp := range a.RelatedParties {
			encryptedDoc, err := ds.Encrypt(&rp.Document)
			if err != nil {
				return err
			}

			amm.RelatedParties[i] = &RelatedPartyMongoDBModel{
				ID:        rp.ID,
				Document:  encryptedDoc,
				Name:      rp.Name,
				Role:      rp.Role,
				StartDate: rp.StartDate,
				EndDate:   rp.EndDate,
			}

			hash := ds.GenerateHash(&rp.Document)
			amm.Search.RelatedPartyDocuments = append(amm.Search.RelatedPartyDocuments, hash)
		}
	}

	if a.Metadata == nil {
		amm.Metadata = make(map[string]any)
	} else {
		amm.Metadata = a.Metadata
	}

	return nil
}

// ToEntity maps a MongoDB model to an Alias entity
func (amm *MongoDBModel) ToEntity(ds *libCrypto.Crypto) (*mmodel.Alias, error) {
	document, err := ds.Decrypt(amm.Document)
	if err != nil {
		return nil, err
	}

	alias := &mmodel.Alias{
		ID:          amm.ID,
		Document:    document,
		Type:        amm.Type,
		LedgerID:    amm.LedgerID,
		AccountID:   amm.AccountID,
		HolderID:    amm.HolderID,
		Metadata:    amm.Metadata,
		ClosingDate: amm.ClosingDate,
		CreatedAt:   *amm.CreatedAt,
		UpdatedAt:   *amm.UpdatedAt,
		DeletedAt:   amm.DeletedAt,
	}

	if amm.BankingDetails != nil {
		accountNumber, err := ds.Decrypt(amm.BankingDetails.Account)
		if err != nil {
			return nil, err
		}

		iban, err := ds.Decrypt(amm.BankingDetails.IBAN)
		if err != nil {
			return nil, err
		}

		alias.BankingDetails = &mmodel.BankingDetails{
			Branch:      amm.BankingDetails.Branch,
			Account:     accountNumber,
			Type:        amm.BankingDetails.Type,
			OpeningDate: amm.BankingDetails.OpeningDate,
			IBAN:        iban,
			CountryCode: amm.BankingDetails.CountryCode,
			BankID:      amm.BankingDetails.BankID,
		}
	}

	if amm.RegulatoryFields != nil {
		participantDocument, err := ds.Decrypt(amm.RegulatoryFields.ParticipantDocument)
		if err != nil {
			return nil, err
		}

		alias.RegulatoryFields = &mmodel.RegulatoryFields{
			ParticipantDocument: participantDocument,
		}
	}

	if len(amm.RelatedParties) > 0 {
		alias.RelatedParties = make([]*mmodel.RelatedParty, len(amm.RelatedParties))

		for i, rp := range amm.RelatedParties {
			decryptedDoc, err := ds.Decrypt(rp.Document)
			if err != nil {
				return nil, err
			}

			docValue := ""
			if decryptedDoc != nil {
				docValue = *decryptedDoc
			}

			alias.RelatedParties[i] = &mmodel.RelatedParty{
				ID:        rp.ID,
				Document:  docValue,
				Name:      rp.Name,
				Role:      rp.Role,
				StartDate: rp.StartDate,
				EndDate:   rp.EndDate,
			}
		}
	}

	return alias, nil
}
