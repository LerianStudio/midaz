// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

import (
	"time"

	libCrypto "github.com/LerianStudio/lib-commons/v3/commons/crypto"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

type MongoDBModel struct {
	ID               *uuid.UUID                    `bson:"_id,omitempty"`
	Document         *string                       `bson:"document,omitempty"`
	Type             *string                       `bson:"type,omitempty"`
	LedgerID         *string                       `bson:"ledger_id,omitempty"`
	AccountID        *string                       `bson:"account_id,omitempty"`
	HolderID         *uuid.UUID                    `bson:"holder_id,omitempty"`
	Metadata         map[string]any                `bson:"metadata"`
	Search           *SearchMongoDB                `bson:"search,omitempty"`
	BankingDetails   *BankingMongoDBModel          `bson:"banking_details,omitempty"`
	RegulatoryFields *RegulatoryFieldsMongoDBModel `bson:"regulatory_fields,omitempty"`
	RelatedParties   []*RelatedPartyMongoDBModel   `bson:"related_parties,omitempty"`
	CreatedAt        *time.Time                    `bson:"created_at,omitempty"`
	UpdatedAt        *time.Time                    `bson:"updated_at"`
	DeletedAt        *time.Time                    `bson:"deleted_at"`
}

type SearchMongoDB struct {
	Document                            *string  `bson:"document,omitempty"`
	BankingDetailsAccount               *string  `bson:"banking_details_account,omitempty"`
	BankingDetailsIBAN                  *string  `bson:"banking_details_iban,omitempty"`
	RegulatoryFieldsParticipantDocument *string  `bson:"regulatory_fields_participant_document,omitempty"`
	RelatedPartyDocuments               []string `bson:"related_party_documents,omitempty"`
}

type BankingMongoDBModel struct {
	Branch      *string    `bson:"branch,omitempty"`
	Account     *string    `bson:"account,omitempty"`
	Type        *string    `bson:"type,omitempty"`
	OpeningDate *string    `bson:"opening_date,omitempty"`
	ClosingDate *time.Time `bson:"closing_date,omitempty"`
	IBAN        *string    `bson:"iban,omitempty"`
	CountryCode *string    `bson:"country_code,omitempty"`
	BankID      *string    `bson:"bank_id,omitempty"`
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

// mapBankingDetailsFromEntity encrypts and maps banking details to MongoDB model.
func mapBankingDetailsFromEntity(bd *mmodel.BankingDetails, ds *libCrypto.Crypto) (*BankingMongoDBModel, *string, *string, error) {
	account, err := ds.Encrypt(bd.Account)
	if err != nil {
		return nil, nil, nil, err
	}

	iban, err := ds.Encrypt(bd.IBAN)
	if err != nil {
		return nil, nil, nil, err
	}

	model := &BankingMongoDBModel{
		Branch:      bd.Branch,
		Account:     account,
		Type:        bd.Type,
		OpeningDate: bd.OpeningDate,
		CountryCode: bd.CountryCode,
		BankID:      bd.BankID,
		IBAN:        iban,
	}

	if bd.ClosingDate != nil {
		model.ClosingDate = &bd.ClosingDate.Time
	}

	var accountHash, ibanHash *string

	if bd.Account != nil && *bd.Account != "" {
		hash := ds.GenerateHash(bd.Account)
		accountHash = &hash
	}

	if bd.IBAN != nil && *bd.IBAN != "" {
		hash := ds.GenerateHash(bd.IBAN)
		ibanHash = &hash
	}

	return model, accountHash, ibanHash, nil
}

// mapRegulatoryFieldsFromEntity encrypts and maps regulatory fields to MongoDB model.
func mapRegulatoryFieldsFromEntity(rf *mmodel.RegulatoryFields, ds *libCrypto.Crypto) (*RegulatoryFieldsMongoDBModel, *string, error) {
	participantDocument, err := ds.Encrypt(rf.ParticipantDocument)
	if err != nil {
		return nil, nil, err
	}

	model := &RegulatoryFieldsMongoDBModel{
		ParticipantDocument: participantDocument,
	}

	var docHash *string

	if rf.ParticipantDocument != nil && *rf.ParticipantDocument != "" {
		hash := ds.GenerateHash(rf.ParticipantDocument)
		docHash = &hash
	}

	return model, docHash, nil
}

// mapRelatedPartiesFromEntity encrypts and maps related parties to MongoDB models.
func mapRelatedPartiesFromEntity(parties []*mmodel.RelatedParty, ds *libCrypto.Crypto) ([]*RelatedPartyMongoDBModel, []string, error) {
	models := make([]*RelatedPartyMongoDBModel, len(parties))
	hashes := make([]string, 0, len(parties))

	for i, rp := range parties {
		encryptedDoc, err := ds.Encrypt(&rp.Document)
		if err != nil {
			return nil, nil, err
		}

		var endDate *time.Time
		if rp.EndDate != nil {
			endDate = &rp.EndDate.Time
		}

		models[i] = &RelatedPartyMongoDBModel{
			ID:        rp.ID,
			Document:  encryptedDoc,
			Name:      rp.Name,
			Role:      rp.Role,
			StartDate: rp.StartDate.Time,
			EndDate:   endDate,
		}

		hash := ds.GenerateHash(&rp.Document)
		hashes = append(hashes, hash)
	}

	return models, hashes, nil
}

// FromEntity maps an account entity to a MongoDB Alias model
func (amm *MongoDBModel) FromEntity(a *mmodel.Alias, ds *libCrypto.Crypto) error {
	document, err := ds.Encrypt(a.Document)
	if err != nil {
		return err
	}

	*amm = MongoDBModel{
		ID:        a.ID,
		Document:  document,
		Type:      a.Type,
		LedgerID:  a.LedgerID,
		AccountID: a.AccountID,
		HolderID:  a.HolderID,
		CreatedAt: &a.CreatedAt,
		UpdatedAt: &a.UpdatedAt,
		DeletedAt: a.DeletedAt,
	}

	amm.Search = &SearchMongoDB{}

	if a.Document != nil && *a.Document != "" {
		hash := ds.GenerateHash(a.Document)
		amm.Search.Document = &hash
	}

	if a.BankingDetails != nil {
		bankingModel, accountHash, ibanHash, err := mapBankingDetailsFromEntity(a.BankingDetails, ds)
		if err != nil {
			return err
		}

		amm.BankingDetails = bankingModel
		amm.Search.BankingDetailsAccount = accountHash
		amm.Search.BankingDetailsIBAN = ibanHash
	}

	if a.RegulatoryFields != nil {
		regulatoryModel, docHash, err := mapRegulatoryFieldsFromEntity(a.RegulatoryFields, ds)
		if err != nil {
			return err
		}

		amm.RegulatoryFields = regulatoryModel
		amm.Search.RegulatoryFieldsParticipantDocument = docHash
	}

	if len(a.RelatedParties) > 0 {
		partiesModels, partiesHashes, err := mapRelatedPartiesFromEntity(a.RelatedParties, ds)
		if err != nil {
			return err
		}

		amm.RelatedParties = partiesModels
		amm.Search.RelatedPartyDocuments = partiesHashes
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
		ID:        amm.ID,
		Document:  document,
		Type:      amm.Type,
		LedgerID:  amm.LedgerID,
		AccountID: amm.AccountID,
		HolderID:  amm.HolderID,
		Metadata:  amm.Metadata,
		CreatedAt: utils.SafeTimePtr(amm.CreatedAt),
		UpdatedAt: utils.SafeTimePtr(amm.UpdatedAt),
		DeletedAt: amm.DeletedAt,
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

		if amm.BankingDetails.ClosingDate != nil {
			alias.BankingDetails.ClosingDate = &mmodel.Date{Time: *amm.BankingDetails.ClosingDate}
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

			var endDate *mmodel.Date
			if rp.EndDate != nil {
				endDate = &mmodel.Date{Time: *rp.EndDate}
			}

			alias.RelatedParties[i] = &mmodel.RelatedParty{
				ID:        rp.ID,
				Document:  docValue,
				Name:      rp.Name,
				Role:      rp.Role,
				StartDate: mmodel.Date{Time: rp.StartDate},
				EndDate:   endDate,
			}
		}
	}

	return alias, nil
}
