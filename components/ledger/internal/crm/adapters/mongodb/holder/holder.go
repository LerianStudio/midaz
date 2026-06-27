// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"time"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	encryption "github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// recordSpanError records err onto the span using the class-appropriate helper:
// business/4xx errors keep the span status UNSET via HandleSpanBusinessErrorEvent,
// technical/5xx errors flip it red via HandleSpanError (telemetry rule T5).
func recordSpanError(span trace.Span, message string, err error) {
	if pkg.IsBusinessError(err) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, message, err)
		return
	}

	libOpentelemetry.HandleSpanError(span, message, err)
}

type MongoDBModel struct {
	ID               *uuid.UUID                 `bson:"_id,omitempty"`
	ExternalID       *string                    `bson:"external_id,omitempty"`
	Type             *string                    `bson:"type,omitempty"`
	Name             *string                    `bson:"name,omitempty"`
	Document         *string                    `bson:"document,omitempty"`
	Addresses        *AddressesMongoDBModel     `bson:"addresses,omitempty"`
	Contact          *ContactMongoDBModel       `bson:"contact,omitempty"`
	NaturalPerson    *NaturalPersonMongoDBModel `bson:"natural_person,omitempty"`
	LegalPerson      *LegalPersonMongoDBModel   `bson:"legal_person,omitempty"`
	Metadata         map[string]any             `bson:"metadata"`
	Search           map[string]string          `bson:"search,omitempty"`
	SearchKeyVersion uint32                     `bson:"search_key_version,omitempty"`
	CreatedAt        *time.Time                 `bson:"created_at,omitempty"`
	UpdatedAt        *time.Time                 `bson:"updated_at"`
	DeletedAt        *time.Time                 `bson:"deleted_at"`
}

type AddressesMongoDBModel struct {
	Primary     *AddressMongoDBModel `bson:"primary,omitempty"`
	Additional1 *AddressMongoDBModel `bson:"additional_1,omitempty"`
	Additional2 *AddressMongoDBModel `bson:"additional_2,omitempty"`
}

type AddressMongoDBModel struct {
	Line1       *string `bson:"line_1,omitempty"`
	Line2       *string `bson:"line_2,omitempty"`
	ZipCode     *string `bson:"zip_code,omitempty"`
	City        *string `bson:"city,omitempty"`
	State       *string `bson:"state,omitempty"`
	Country     *string `bson:"country,omitempty"`
	Description *string `bson:"description,omitempty"`
	IsPrimary   *bool   `bson:"is_primary,omitempty"`
}

type ContactMongoDBModel struct {
	PrimaryEmail   *string `bson:"primary_email,omitempty"`
	SecondaryEmail *string `bson:"secondary_email,omitempty"`
	MobilePhone    *string `bson:"mobile_phone,omitempty"`
	OtherPhone     *string `bson:"other_phone,omitempty"`
}

type NaturalPersonMongoDBModel struct {
	FavoriteName *string `bson:"favorite_name,omitempty"`
	SocialName   *string `bson:"social_name,omitempty"`
	Gender       *string `bson:"gender,omitempty"`
	BirthDate    *string `bson:"birth_date,omitempty"`
	CivilStatus  *string `bson:"civil_status,omitempty"`
	Nationality  *string `bson:"nationality,omitempty"`
	MotherName   *string `bson:"mother_name,omitempty"`
	FatherName   *string `bson:"father_name,omitempty"`
	Status       *string `bson:"status,omitempty"`
}

type LegalPersonMongoDBModel struct {
	TradeName      *string                     `bson:"trade_name,omitempty"`
	Activity       *string                     `bson:"activity,omitempty"`
	Type           *string                     `bson:"type,omitempty"`
	FoundingDate   *time.Time                  `bson:"founding_date,omitempty"`
	Size           *string                     `bson:"size,omitempty"`
	Status         *string                     `bson:"status,omitempty"`
	Representative *RepresentativeMongoDBModel `bson:"representative,omitempty"`
}

type RepresentativeMongoDBModel struct {
	Name     *string `bson:"name,omitempty"`
	Document *string `bson:"document,omitempty"`
	Email    *string `bson:"email,omitempty"`
	Role     *string `bson:"role,omitempty"`
}

// FromEntity maps a holder entity to a MongoDB Holder model.
// It uses FieldEncryptor for encrypting sensitive fields with the provided EncryptionContext.
func (hmm *MongoDBModel) FromEntity(ctx context.Context, h *mmodel.Holder, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext) error {
	fieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
	}

	*hmm = MongoDBModel{
		ID:         h.ID,
		ExternalID: h.ExternalID,
		Type:       h.Type,
		CreatedAt:  &h.CreatedAt,
		UpdatedAt:  &h.UpdatedAt,
		DeletedAt:  h.DeletedAt,
	}

	if h.Name != nil {
		fieldCtx.FieldName = "name"

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *h.Name)
		if err != nil {
			return err
		}

		hmm.Name = &encrypted
	}

	if h.Document != nil {
		fieldCtx.FieldName = "document"

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *h.Document)
		if err != nil {
			return err
		}

		hmm.Document = &encrypted
	}

	if h.Addresses != nil {
		hmm.Addresses = mapAddressesFromEntity(h.Addresses)
	}

	if h.Contact != nil {
		contact, err := mapContactFromEntity(ctx, fe, encryptionCtx, h.Contact)
		if err != nil {
			return err
		}

		hmm.Contact = contact
	}

	if h.NaturalPerson != nil {
		naturalPerson, err := mapNaturalPersonFromEntity(ctx, fe, encryptionCtx, h.NaturalPerson)
		if err != nil {
			return err
		}

		hmm.NaturalPerson = naturalPerson
	}

	if h.LegalPerson != nil {
		legalPerson, err := mapLegalPersonFromEntity(ctx, fe, encryptionCtx, h.LegalPerson)
		if err != nil {
			return err
		}

		hmm.LegalPerson = legalPerson
	}

	// Generate search token for document field
	hmm.Search = make(map[string]string)

	if h.Document != nil && *h.Document != "" {
		searchCtx := encryption.SearchTokenContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			FieldName:      "document",
		}

		searchToken, keyVersion, tokenErr := fe.GenerateSearchToken(ctx, searchCtx, *h.Document)
		if tokenErr != nil {
			return tokenErr
		}

		hmm.Search["document"] = searchToken
		// Holder has a single search field, so the version is assigned directly here;
		// alias has multiple search fields and uses a first-non-zero-wins helper instead.
		hmm.SearchKeyVersion = keyVersion
	}

	if h.Metadata == nil {
		hmm.Metadata = make(map[string]any)
	} else {
		hmm.Metadata = h.Metadata
	}

	return nil
}

// mapAddressesFromEntity maps addresses entity to MongoDB model
func mapAddressesFromEntity(a *mmodel.Addresses) *AddressesMongoDBModel {
	return &AddressesMongoDBModel{
		Primary:     mapAddressFromEntity(a.Primary),
		Additional1: mapAddressFromEntity(a.Additional1),
		Additional2: mapAddressFromEntity(a.Additional2),
	}
}

// mapContactFromEntity maps contact entity to MongoDB model with field encryption
func mapContactFromEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, c *mmodel.Contact) (*ContactMongoDBModel, error) {
	fieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
	}

	contact := &ContactMongoDBModel{}

	if c.PrimaryEmail != nil {
		fieldCtx.FieldName = "contact.primary_email"

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *c.PrimaryEmail)
		if err != nil {
			return nil, err
		}

		contact.PrimaryEmail = &encrypted
	}

	if c.SecondaryEmail != nil {
		fieldCtx.FieldName = "contact.secondary_email"

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *c.SecondaryEmail)
		if err != nil {
			return nil, err
		}

		contact.SecondaryEmail = &encrypted
	}

	if c.MobilePhone != nil {
		fieldCtx.FieldName = "contact.mobile_phone"

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *c.MobilePhone)
		if err != nil {
			return nil, err
		}

		contact.MobilePhone = &encrypted
	}

	if c.OtherPhone != nil {
		fieldCtx.FieldName = "contact.other_phone"

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *c.OtherPhone)
		if err != nil {
			return nil, err
		}

		contact.OtherPhone = &encrypted
	}

	return contact, nil
}

// mapNaturalPersonFromEntity maps natural person entity to MongoDB model with field encryption
func mapNaturalPersonFromEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, np *mmodel.NaturalPerson) (*NaturalPersonMongoDBModel, error) {
	fieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
	}

	result := &NaturalPersonMongoDBModel{
		FavoriteName: np.FavoriteName,
		SocialName:   np.SocialName,
		Gender:       np.Gender,
		BirthDate:    np.BirthDate,
		CivilStatus:  np.CivilStatus,
		Nationality:  np.Nationality,
		Status:       np.Status,
	}

	if np.MotherName != nil {
		fieldCtx.FieldName = "natural_person.mother_name"

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *np.MotherName)
		if err != nil {
			return nil, err
		}

		result.MotherName = &encrypted
	}

	if np.FatherName != nil {
		fieldCtx.FieldName = "natural_person.father_name"

		encrypted, err := fe.EncryptField(ctx, fieldCtx, *np.FatherName)
		if err != nil {
			return nil, err
		}

		result.FatherName = &encrypted
	}

	return result, nil
}

// mapLegalPersonFromEntity maps legal person entity to MongoDB model with field encryption
func mapLegalPersonFromEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, lp *mmodel.LegalPerson) (*LegalPersonMongoDBModel, error) {
	var parsedFoundingDate *time.Time

	if lp.FoundingDate != nil {
		parsed, err := time.Parse("2006-01-02", *lp.FoundingDate)
		if err != nil {
			return nil, err
		}

		parsedFoundingDate = &parsed
	}

	mongoLP := &LegalPersonMongoDBModel{
		TradeName:    lp.TradeName,
		Activity:     lp.Activity,
		Type:         lp.Type,
		FoundingDate: parsedFoundingDate,
		Status:       lp.Status,
		Size:         lp.Size,
	}

	if lp.Representative != nil {
		fieldCtx := encryption.FieldContext{
			TenantID:       encryptionCtx.TenantID,
			OrganizationID: encryptionCtx.OrganizationID,
			RecordID:       encryptionCtx.RecordID,
		}

		rep := &RepresentativeMongoDBModel{
			Role: lp.Representative.Role,
		}

		if lp.Representative.Name != nil {
			fieldCtx.FieldName = "legal_person.representative.name"

			encrypted, err := fe.EncryptField(ctx, fieldCtx, *lp.Representative.Name)
			if err != nil {
				return nil, err
			}

			rep.Name = &encrypted
		}

		if lp.Representative.Document != nil {
			fieldCtx.FieldName = "legal_person.representative.document"

			encrypted, err := fe.EncryptField(ctx, fieldCtx, *lp.Representative.Document)
			if err != nil {
				return nil, err
			}

			rep.Document = &encrypted
		}

		if lp.Representative.Email != nil {
			fieldCtx.FieldName = "legal_person.representative.email"

			encrypted, err := fe.EncryptField(ctx, fieldCtx, *lp.Representative.Email)
			if err != nil {
				return nil, err
			}

			rep.Email = &encrypted
		}

		mongoLP.Representative = rep
	}

	return mongoLP, nil
}

// mapAddressFromEntity maps an address entity to MongoDB model
func mapAddressFromEntity(a *mmodel.Address) *AddressMongoDBModel {
	if a == nil {
		return nil
	}

	return &AddressMongoDBModel{
		Line1:       &a.Line1,
		Line2:       a.Line2,
		ZipCode:     &a.ZipCode,
		City:        &a.City,
		State:       &a.State,
		Country:     &a.Country,
		Description: a.Description,
	}
}

// ToEntity maps a MongoDB model to a Holder entity.
// It uses FieldEncryptor for decrypting sensitive fields with the provided EncryptionContext.
func (hmm *MongoDBModel) ToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext) (*mmodel.Holder, error) {
	fieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
	}

	holder := &mmodel.Holder{
		ID:         hmm.ID,
		ExternalID: hmm.ExternalID,
		Type:       hmm.Type,
		Metadata:   hmm.Metadata,
		CreatedAt:  utils.SafeTimePtr(hmm.CreatedAt),
		UpdatedAt:  utils.SafeTimePtr(hmm.UpdatedAt),
		DeletedAt:  hmm.DeletedAt,
	}

	if hmm.Name != nil {
		fieldCtx.FieldName = "name"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *hmm.Name)
		if err != nil {
			return nil, err
		}

		holder.Name = &decrypted
	}

	if hmm.Document != nil {
		fieldCtx.FieldName = "document"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *hmm.Document)
		if err != nil {
			return nil, err
		}

		holder.Document = &decrypted
	}

	if hmm.Addresses != nil {
		holder.Addresses = mapAddressesToEntity(hmm.Addresses)
	}

	if hmm.Contact != nil {
		contact, contactErr := mapContactToEntity(ctx, fe, encryptionCtx, hmm.Contact)
		if contactErr != nil {
			return nil, contactErr
		}

		holder.Contact = contact
	}

	if hmm.NaturalPerson != nil {
		np, npErr := mapNaturalPersonToEntity(ctx, fe, encryptionCtx, hmm.NaturalPerson)
		if npErr != nil {
			return nil, npErr
		}

		holder.NaturalPerson = np
	}

	if hmm.LegalPerson != nil {
		lp, lpErr := mapLegalPersonToEntity(ctx, fe, encryptionCtx, hmm.LegalPerson)
		if lpErr != nil {
			return nil, lpErr
		}

		holder.LegalPerson = lp
	}

	return holder, nil
}

// mapAddressesToEntity maps a MongoDB model to an Addresses entity
func mapAddressesToEntity(a *AddressesMongoDBModel) *mmodel.Addresses {
	return &mmodel.Addresses{
		Primary:     mapAddressToEntity(a.Primary),
		Additional1: mapAddressToEntity(a.Additional1),
		Additional2: mapAddressToEntity(a.Additional2),
	}
}

// mapContactToEntity maps a MongoDB model to a Contact entity with field decryption
func mapContactToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, c *ContactMongoDBModel) (*mmodel.Contact, error) {
	fieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
	}

	contact := &mmodel.Contact{}

	if c.PrimaryEmail != nil {
		fieldCtx.FieldName = "contact.primary_email"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *c.PrimaryEmail)
		if err != nil {
			return nil, err
		}

		contact.PrimaryEmail = &decrypted
	}

	if c.SecondaryEmail != nil {
		fieldCtx.FieldName = "contact.secondary_email"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *c.SecondaryEmail)
		if err != nil {
			return nil, err
		}

		contact.SecondaryEmail = &decrypted
	}

	if c.MobilePhone != nil {
		fieldCtx.FieldName = "contact.mobile_phone"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *c.MobilePhone)
		if err != nil {
			return nil, err
		}

		contact.MobilePhone = &decrypted
	}

	if c.OtherPhone != nil {
		fieldCtx.FieldName = "contact.other_phone"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *c.OtherPhone)
		if err != nil {
			return nil, err
		}

		contact.OtherPhone = &decrypted
	}

	return contact, nil
}

// mapNaturalPersonToEntity maps a MongoDB model to a NaturalPerson entity with field decryption
func mapNaturalPersonToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, np *NaturalPersonMongoDBModel) (*mmodel.NaturalPerson, error) {
	fieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
	}

	result := &mmodel.NaturalPerson{
		FavoriteName: np.FavoriteName,
		SocialName:   np.SocialName,
		Gender:       np.Gender,
		BirthDate:    np.BirthDate,
		CivilStatus:  np.CivilStatus,
		Nationality:  np.Nationality,
		Status:       np.Status,
	}

	if np.MotherName != nil {
		fieldCtx.FieldName = "natural_person.mother_name"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *np.MotherName)
		if err != nil {
			return nil, err
		}

		result.MotherName = &decrypted
	}

	if np.FatherName != nil {
		fieldCtx.FieldName = "natural_person.father_name"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *np.FatherName)
		if err != nil {
			return nil, err
		}

		result.FatherName = &decrypted
	}

	return result, nil
}

// mapLegalPersonToEntity maps a MongoDB model to a LegalPerson entity with field decryption
func mapLegalPersonToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, lp *LegalPersonMongoDBModel) (*mmodel.LegalPerson, error) {
	var foundingDate *string

	if lp.FoundingDate != nil {
		formatted := lp.FoundingDate.Format("2006-01-02")
		foundingDate = &formatted
	}

	legalPerson := &mmodel.LegalPerson{
		TradeName:    lp.TradeName,
		Activity:     lp.Activity,
		Type:         lp.Type,
		FoundingDate: foundingDate,
		Status:       lp.Status,
		Size:         lp.Size,
	}

	if lp.Representative != nil {
		rep, err := mapRepresentativeToEntity(ctx, fe, encryptionCtx, lp.Representative)
		if err != nil {
			return nil, err
		}

		legalPerson.Representative = rep
	}

	return legalPerson, nil
}

// mapRepresentativeToEntity maps a MongoDB model to a Representative entity with field decryption
func mapRepresentativeToEntity(ctx context.Context, fe encryption.FieldEncryptor, encryptionCtx encryption.EncryptionContext, rep *RepresentativeMongoDBModel) (*mmodel.Representative, error) {
	fieldCtx := encryption.FieldContext{
		TenantID:       encryptionCtx.TenantID,
		OrganizationID: encryptionCtx.OrganizationID,
		RecordID:       encryptionCtx.RecordID,
	}

	result := &mmodel.Representative{
		Role: rep.Role,
	}

	if rep.Name != nil {
		fieldCtx.FieldName = "legal_person.representative.name"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *rep.Name)
		if err != nil {
			return nil, err
		}

		result.Name = &decrypted
	}

	if rep.Document != nil {
		fieldCtx.FieldName = "legal_person.representative.document"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *rep.Document)
		if err != nil {
			return nil, err
		}

		result.Document = &decrypted
	}

	if rep.Email != nil {
		fieldCtx.FieldName = "legal_person.representative.email"

		decrypted, err := fe.DecryptField(ctx, fieldCtx, *rep.Email)
		if err != nil {
			return nil, err
		}

		result.Email = &decrypted
	}

	return result, nil
}

// mapAddressToEntity maps a MongoDB model to an Address entity
func mapAddressToEntity(a *AddressMongoDBModel) *mmodel.Address {
	if a == nil {
		return nil
	}

	var line1, zipCode, city, state, country string
	if a.Line1 != nil {
		line1 = *a.Line1
	}

	if a.ZipCode != nil {
		zipCode = *a.ZipCode
	}

	if a.City != nil {
		city = *a.City
	}

	if a.State != nil {
		state = *a.State
	}

	if a.Country != nil {
		country = *a.Country
	}

	return &mmodel.Address{
		Line1:       line1,
		Line2:       a.Line2,
		ZipCode:     zipCode,
		City:        city,
		State:       state,
		Country:     country,
		Description: a.Description,
	}
}
