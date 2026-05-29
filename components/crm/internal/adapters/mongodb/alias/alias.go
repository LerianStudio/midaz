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
func mapBankingDetailsFromEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, bd *mmodel.BankingDetails) (*BankingMongoDBModel, *string, *string, error) {
	accountFieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
		FieldName:      "banking_details.account",
	}

	account, err := fe.EncryptOptional(ctx, accountFieldCtx, bd.Account)
	if err != nil {
		return nil, nil, nil, err
	}

	ibanFieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
		FieldName:      "banking_details.iban",
	}

	iban, err := fe.EncryptOptional(ctx, ibanFieldCtx, bd.IBAN)
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
		accountSearchCtx := encryption.SearchTokenContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			FieldName:      "banking_details.account",
		}

		hash, hashErr := fe.GenerateSearchToken(ctx, accountSearchCtx, *bd.Account)
		if hashErr != nil {
			return nil, nil, nil, hashErr
		}

		accountHash = &hash
	}

	if bd.IBAN != nil && *bd.IBAN != "" {
		ibanSearchCtx := encryption.SearchTokenContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			FieldName:      "banking_details.iban",
		}

		hash, hashErr := fe.GenerateSearchToken(ctx, ibanSearchCtx, *bd.IBAN)
		if hashErr != nil {
			return nil, nil, nil, hashErr
		}

		ibanHash = &hash
	}

	return model, accountHash, ibanHash, nil
}

// mapBankingDetailsToEntity decrypts and maps banking details from MongoDB model.
func mapBankingDetailsToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, bd *BankingMongoDBModel) (*mmodel.BankingDetails, error) {
	accountFieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
		FieldName:      "banking_details.account",
	}

	accountNumber, err := fe.DecryptOptional(ctx, accountFieldCtx, bd.Account)
	if err != nil {
		return nil, err
	}

	ibanFieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
		FieldName:      "banking_details.iban",
	}

	iban, err := fe.DecryptOptional(ctx, ibanFieldCtx, bd.IBAN)
	if err != nil {
		return nil, err
	}

	bankingDetails := &mmodel.BankingDetails{
		Branch:      bd.Branch,
		Account:     accountNumber,
		Type:        bd.Type,
		OpeningDate: bd.OpeningDate,
		IBAN:        iban,
		CountryCode: bd.CountryCode,
		BankID:      bd.BankID,
	}

	if bd.ClosingDate != nil {
		bankingDetails.ClosingDate = &mmodel.Date{Time: *bd.ClosingDate}
	}

	return bankingDetails, nil
}

// mapRegulatoryFieldsFromEntity encrypts and maps regulatory fields to MongoDB model.
func mapRegulatoryFieldsFromEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, rf *mmodel.RegulatoryFields) (*RegulatoryFieldsMongoDBModel, *string, error) {
	participantDocFieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
		FieldName:      "regulatory_fields.participant_document",
	}

	participantDocument, err := fe.EncryptOptional(ctx, participantDocFieldCtx, rf.ParticipantDocument)
	if err != nil {
		return nil, nil, err
	}

	model := &RegulatoryFieldsMongoDBModel{
		ParticipantDocument: participantDocument,
	}

	var docHash *string

	if rf.ParticipantDocument != nil && *rf.ParticipantDocument != "" {
		searchCtx := encryption.SearchTokenContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			FieldName:      "regulatory_fields.participant_document",
		}

		hash, hashErr := fe.GenerateSearchToken(ctx, searchCtx, *rf.ParticipantDocument)
		if hashErr != nil {
			return nil, nil, hashErr
		}

		docHash = &hash
	}

	return model, docHash, nil
}

// mapRegulatoryFieldsToEntity decrypts and maps regulatory fields from MongoDB model.
func mapRegulatoryFieldsToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, rf *RegulatoryFieldsMongoDBModel) (*mmodel.RegulatoryFields, error) {
	participantDocFieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
		FieldName:      "regulatory_fields.participant_document",
	}

	participantDocument, err := fe.DecryptOptional(ctx, participantDocFieldCtx, rf.ParticipantDocument)
	if err != nil {
		return nil, err
	}

	return &mmodel.RegulatoryFields{
		ParticipantDocument: participantDocument,
	}, nil
}

// mapRelatedPartiesFromEntity encrypts and maps related parties to MongoDB models.
func mapRelatedPartiesFromEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, parties []*mmodel.RelatedParty) ([]*RelatedPartyMongoDBModel, []string, error) {
	models := make([]*RelatedPartyMongoDBModel, len(parties))
	hashes := make([]string, 0, len(parties))

	for i, rp := range parties {
		docFieldCtx := encryption.FieldContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			RecordID:       encryptionCtx.RecordID,
			FieldName:      fmt.Sprintf("related_parties.%d.document", i),
		}

		encryptedDoc, err := fe.EncryptField(ctx, docFieldCtx, rp.Document)
		if err != nil {
			return nil, nil, err
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

		hash, hashErr := fe.GenerateSearchToken(ctx, searchCtx, rp.Document)
		if hashErr != nil {
			return nil, nil, hashErr
		}

		hashes = append(hashes, hash)
	}

	return models, hashes, nil
}

// mapRelatedPartiesToEntity decrypts and maps related parties from MongoDB models.
func mapRelatedPartiesToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, parties []*RelatedPartyMongoDBModel) ([]*mmodel.RelatedParty, error) {
	result := make([]*mmodel.RelatedParty, len(parties))

	for i, rp := range parties {
		docFieldCtx := encryption.FieldContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			RecordID:       encryptionCtx.RecordID,
			FieldName:      fmt.Sprintf("related_parties.%d.document", i),
		}

		decryptedDoc, err := fe.DecryptOptional(ctx, docFieldCtx, rp.Document)
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
	documentFieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
		FieldName:      "document",
	}

	document, err := fe.EncryptOptional(ctx, documentFieldCtx, a.Document)
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

	// Generate search token for document field
	if a.Document != nil && *a.Document != "" {
		searchCtx := encryption.SearchTokenContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			FieldName:      "document",
		}

		hash, hashErr := fe.GenerateSearchToken(ctx, searchCtx, *a.Document)
		if hashErr != nil {
			return hashErr
		}

		amm.Search.Document = &hash
	}

	if a.BankingDetails != nil {
		bankingModel, accountHash, ibanHash, bankingErr := mapBankingDetailsFromEntity(ctx, fe, encryptionCtx, a.BankingDetails)
		if bankingErr != nil {
			return bankingErr
		}

		amm.BankingDetails = bankingModel
		amm.Search.BankingDetailsAccount = accountHash
		amm.Search.BankingDetailsIBAN = ibanHash
	}

	if a.RegulatoryFields != nil {
		regulatoryModel, docHash, regErr := mapRegulatoryFieldsFromEntity(ctx, fe, encryptionCtx, a.RegulatoryFields)
		if regErr != nil {
			return regErr
		}

		amm.RegulatoryFields = regulatoryModel
		amm.Search.RegulatoryFieldsParticipantDocument = docHash
	}

	if len(a.RelatedParties) > 0 {
		partiesModels, partiesHashes, partiesErr := mapRelatedPartiesFromEntity(ctx, fe, encryptionCtx, a.RelatedParties)
		if partiesErr != nil {
			return partiesErr
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

// ToEntity maps a MongoDB model to an Alias entity.
// It uses FieldEncryptor for decrypting sensitive fields with the provided EncryptionContext.
func (amm *MongoDBModel) ToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext) (*mmodel.Alias, error) {
	documentFieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
		FieldName:      "document",
	}

	document, err := fe.DecryptOptional(ctx, documentFieldCtx, amm.Document)
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
