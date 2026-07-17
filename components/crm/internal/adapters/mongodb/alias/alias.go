// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package alias

import (
	"context"
	"fmt"
	"time"

	"github.com/LerianStudio/midaz/v3/components/crm/internal/services/encryption"
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
	SearchKeyVersion uint32                        `bson:"search_key_version,omitempty"`
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

// stampSearchKeyVersion records the PRF keyset primary version used for the document's
// search tokens. All of a document's tokens share one version (same org primary), so the
// first non-zero version observed is kept; a legacy write (version 0) leaves it unset.
func (amm *MongoDBModel) stampSearchKeyVersion(keyVersion uint32) {
	if amm.SearchKeyVersion == 0 {
		amm.SearchKeyVersion = keyVersion
	}
}

// mapBankingDetailsFromEntity encrypts and maps banking details to MongoDB model.
// The returned key version is the PRF keyset primary used for the generated search tokens.
func mapBankingDetailsFromEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, bd *mmodel.BankingDetails) (*BankingMongoDBModel, *string, *string, uint32, error) {
	fieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
	}

	model := &BankingMongoDBModel{
		Branch:      bd.Branch,
		Type:        bd.Type,
		OpeningDate: bd.OpeningDate,
		CountryCode: bd.CountryCode,
		BankID:      bd.BankID,
	}

	if bd.ClosingDate != nil {
		model.ClosingDate = &bd.ClosingDate.Time
	}

	var (
		accountHash, ibanHash *string
		keyVersion            uint32
	)

	if bd.Account != nil {
		fieldCtx.FieldName = "banking_details.account"

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *bd.Account)
		if err != nil {
			return nil, nil, nil, 0, err
		}

		model.Account = &encrypted

		if *bd.Account != "" {
			searchCtx := encryption.SearchTokenContext{
				TenantID:       encryptionCtx.TenantID,
				OrganizationID: encryptionCtx.OrganizationID,
				FieldName:      "banking_details.account",
			}

			hash, version, hashErr := fe.GenerateSearchToken(ctx, searchCtx, *bd.Account)
			if hashErr != nil {
				return nil, nil, nil, 0, hashErr
			}

			accountHash = &hash
			keyVersion = version
		}
	}

	if bd.IBAN != nil {
		fieldCtx.FieldName = "banking_details.iban"

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *bd.IBAN)
		if err != nil {
			return nil, nil, nil, 0, err
		}

		model.IBAN = &encrypted

		if *bd.IBAN != "" {
			searchCtx := encryption.SearchTokenContext{
				TenantID:       encryptionCtx.TenantID,
				OrganizationID: encryptionCtx.OrganizationID,
				FieldName:      "banking_details.iban",
			}

			hash, version, hashErr := fe.GenerateSearchToken(ctx, searchCtx, *bd.IBAN)
			if hashErr != nil {
				return nil, nil, nil, 0, hashErr
			}

			ibanHash = &hash
			keyVersion = version
		}
	}

	return model, accountHash, ibanHash, keyVersion, nil
}

// mapBankingDetailsToEntity decrypts and maps banking details from MongoDB model.
func mapBankingDetailsToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, bd *BankingMongoDBModel) (*mmodel.BankingDetails, error) {
	fieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
	}

	bankingDetails := &mmodel.BankingDetails{
		Branch:      bd.Branch,
		Type:        bd.Type,
		OpeningDate: bd.OpeningDate,
		CountryCode: bd.CountryCode,
		BankID:      bd.BankID,
	}

	if bd.Account != nil {
		fieldCtx.FieldName = "banking_details.account"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *bd.Account)
		if err != nil {
			return nil, err
		}

		bankingDetails.Account = &decrypted
	}

	if bd.IBAN != nil {
		fieldCtx.FieldName = "banking_details.iban"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *bd.IBAN)
		if err != nil {
			return nil, err
		}

		bankingDetails.IBAN = &decrypted
	}

	if bd.ClosingDate != nil {
		bankingDetails.ClosingDate = &mmodel.Date{Time: *bd.ClosingDate}
	}

	return bankingDetails, nil
}

// mapRegulatoryFieldsFromEntity encrypts and maps regulatory fields to MongoDB model.
// The returned key version is the PRF keyset primary used for the generated search token.
func mapRegulatoryFieldsFromEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, rf *mmodel.RegulatoryFields) (*RegulatoryFieldsMongoDBModel, *string, uint32, error) {
	model := &RegulatoryFieldsMongoDBModel{}

	var (
		docHash    *string
		keyVersion uint32
	)

	if rf.ParticipantDocument != nil {
		fieldCtx := encryption.FieldContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			RecordID:       encryptionCtx.RecordID,
			FieldName:      "regulatory_fields.participant_document",
		}

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *rf.ParticipantDocument)
		if err != nil {
			return nil, nil, 0, err
		}

		model.ParticipantDocument = &encrypted

		if *rf.ParticipantDocument != "" {
			searchCtx := encryption.SearchTokenContext{
				TenantID:       encryptionCtx.TenantID,
				OrganizationID: encryptionCtx.OrganizationID,
				FieldName:      "regulatory_fields.participant_document",
			}

			hash, version, hashErr := fe.GenerateSearchToken(ctx, searchCtx, *rf.ParticipantDocument)
			if hashErr != nil {
				return nil, nil, 0, hashErr
			}

			docHash = &hash
			keyVersion = version
		}
	}

	return model, docHash, keyVersion, nil
}

// mapRegulatoryFieldsToEntity decrypts and maps regulatory fields from MongoDB model.
func mapRegulatoryFieldsToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, rf *RegulatoryFieldsMongoDBModel) (*mmodel.RegulatoryFields, error) {
	result := &mmodel.RegulatoryFields{}

	if rf.ParticipantDocument != nil {
		fieldCtx := encryption.FieldContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			RecordID:       encryptionCtx.RecordID,
			FieldName:      "regulatory_fields.participant_document",
		}

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *rf.ParticipantDocument)
		if err != nil {
			return nil, err
		}

		result.ParticipantDocument = &decrypted
	}

	return result, nil
}

// mapRelatedPartiesFromEntity encrypts and maps related parties to MongoDB models.
// The returned key version is the PRF keyset primary used for the generated search tokens.
func mapRelatedPartiesFromEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, parties []*mmodel.RelatedParty) ([]*RelatedPartyMongoDBModel, []string, uint32, error) {
	models := make([]*RelatedPartyMongoDBModel, len(parties))
	hashes := make([]string, 0, len(parties))

	var keyVersion uint32

	for i, rp := range parties {
		docFieldCtx := encryption.FieldContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			RecordID:       encryptionCtx.RecordID,
			FieldName:      fmt.Sprintf("related_parties.%s.document", rp.ID),
		}

		encryptedDoc, err := fe.EncryptField(ctx, docFieldCtx, rp.Document)
		if err != nil {
			return nil, nil, 0, err
		}

		var endDate *time.Time
		if rp.EndDate != nil {
			endDate = &rp.EndDate.Time
		}

		models[i] = &RelatedPartyMongoDBModel{
			ID:        rp.ID,
			Document:  &encryptedDoc,
			Name:      rp.Name,
			Role:      rp.Role,
			StartDate: rp.StartDate.Time,
			EndDate:   endDate,
		}

		searchCtx := encryption.SearchTokenContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			FieldName:      "related_parties.document",
		}

		hash, version, hashErr := fe.GenerateSearchToken(ctx, searchCtx, rp.Document)
		if hashErr != nil {
			return nil, nil, 0, hashErr
		}

		hashes = append(hashes, hash)
		keyVersion = version
	}

	return models, hashes, keyVersion, nil
}

// mapRelatedPartiesToEntity decrypts and maps related parties from MongoDB models.
func mapRelatedPartiesToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, parties []*RelatedPartyMongoDBModel) ([]*mmodel.RelatedParty, error) {
	result := make([]*mmodel.RelatedParty, len(parties))

	for i, rp := range parties {
		var docValue string

		if rp.Document != nil {
			fieldCtx := encryption.FieldContext{
				TenantID:       encryptionCtx.TenantID,
				OrganizationID: encryptionCtx.OrganizationID,
				RecordID:       encryptionCtx.RecordID,
				FieldName:      fmt.Sprintf("related_parties.%s.document", rp.ID),
			}

			decrypted, err := fe.DecryptField(ctx, fieldCtx, *rp.Document)
			if err != nil {
				return nil, err
			}

			docValue = decrypted
		}

		var endDate *mmodel.Date
		if rp.EndDate != nil {
			endDate = &mmodel.Date{Time: *rp.EndDate}
		}

		result[i] = &mmodel.RelatedParty{
			ID:        rp.ID,
			Document:  docValue,
			Name:      rp.Name,
			Role:      rp.Role,
			StartDate: mmodel.Date{Time: rp.StartDate},
			EndDate:   endDate,
		}
	}

	return result, nil
}

// FromEntity maps an alias entity to a MongoDB Alias model.
// It uses FieldEncryptor for encrypting sensitive fields with the provided EncryptionContext.
func (amm *MongoDBModel) FromEntity(ctx context.Context, a *mmodel.Alias, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext) error {
	*amm = MongoDBModel{
		ID:        a.ID,
		Type:      a.Type,
		LedgerID:  a.LedgerID,
		AccountID: a.AccountID,
		HolderID:  a.HolderID,
		CreatedAt: &a.CreatedAt,
		UpdatedAt: &a.UpdatedAt,
		DeletedAt: a.DeletedAt,
	}

	amm.Search = &SearchMongoDB{}

	if a.Document != nil {
		fieldCtx := encryption.FieldContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			RecordID:       encryptionCtx.RecordID,
			FieldName:      "document",
		}

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *a.Document)
		if err != nil {
			return err
		}

		amm.Document = &encrypted

		// Generate search token for document field
		if *a.Document != "" {
			searchCtx := encryption.SearchTokenContext{
				TenantID:       encryptionCtx.TenantID,
				OrganizationID: encryptionCtx.OrganizationID,
				FieldName:      "document",
			}

			hash, keyVersion, hashErr := fe.GenerateSearchToken(ctx, searchCtx, *a.Document)
			if hashErr != nil {
				return hashErr
			}

			amm.Search.Document = &hash
			amm.stampSearchKeyVersion(keyVersion)
		}
	}

	if a.BankingDetails != nil {
		bankingModel, accountHash, ibanHash, keyVersion, bankingErr := mapBankingDetailsFromEntity(ctx, fe, encryptionCtx, a.BankingDetails)
		if bankingErr != nil {
			return bankingErr
		}

		amm.BankingDetails = bankingModel
		amm.Search.BankingDetailsAccount = accountHash
		amm.Search.BankingDetailsIBAN = ibanHash
		amm.stampSearchKeyVersion(keyVersion)
	}

	if a.RegulatoryFields != nil {
		regulatoryModel, docHash, keyVersion, regErr := mapRegulatoryFieldsFromEntity(ctx, fe, encryptionCtx, a.RegulatoryFields)
		if regErr != nil {
			return regErr
		}

		amm.RegulatoryFields = regulatoryModel
		amm.Search.RegulatoryFieldsParticipantDocument = docHash
		amm.stampSearchKeyVersion(keyVersion)
	}

	if len(a.RelatedParties) > 0 {
		partiesModels, partiesHashes, keyVersion, partiesErr := mapRelatedPartiesFromEntity(ctx, fe, encryptionCtx, a.RelatedParties)
		if partiesErr != nil {
			return partiesErr
		}

		amm.RelatedParties = partiesModels
		amm.Search.RelatedPartyDocuments = partiesHashes
		amm.stampSearchKeyVersion(keyVersion)
	}

	if a.Metadata == nil {
		amm.Metadata = make(map[string]any)
	} else {
		amm.Metadata = a.Metadata
	}

	return nil
}

// ToEntity maps a MongoDB model to an Alias entity.
// It uses FieldEncryptor for decrypting sensitive fields with the provided EncryptionContext.
func (amm *MongoDBModel) ToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext) (*mmodel.Alias, error) {
	alias := &mmodel.Alias{
		ID:        amm.ID,
		Type:      amm.Type,
		LedgerID:  amm.LedgerID,
		AccountID: amm.AccountID,
		HolderID:  amm.HolderID,
		Metadata:  amm.Metadata,
		CreatedAt: utils.SafeTimePtr(amm.CreatedAt),
		UpdatedAt: utils.SafeTimePtr(amm.UpdatedAt),
		DeletedAt: amm.DeletedAt,
	}

	if amm.Document != nil {
		fieldCtx := encryption.FieldContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			RecordID:       encryptionCtx.RecordID,
			FieldName:      "document",
		}

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *amm.Document)
		if err != nil {
			return nil, err
		}

		alias.Document = &decrypted
	}

	if amm.BankingDetails != nil {
		bankingDetails, bankingErr := mapBankingDetailsToEntity(ctx, fe, encryptionCtx, amm.BankingDetails)
		if bankingErr != nil {
			return nil, bankingErr
		}

		alias.BankingDetails = bankingDetails
	}

	if amm.RegulatoryFields != nil {
		regulatoryFields, regErr := mapRegulatoryFieldsToEntity(ctx, fe, encryptionCtx, amm.RegulatoryFields)
		if regErr != nil {
			return nil, regErr
		}

		alias.RegulatoryFields = regulatoryFields
	}

	if len(amm.RelatedParties) > 0 {
		relatedParties, partiesErr := mapRelatedPartiesToEntity(ctx, fe, encryptionCtx, amm.RelatedParties)
		if partiesErr != nil {
			return nil, partiesErr
		}

		alias.RelatedParties = relatedParties
	}

	return alias, nil
}
