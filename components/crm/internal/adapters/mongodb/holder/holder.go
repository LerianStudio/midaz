// Package holder provides MongoDB repository implementation for holder entities.
package holder

import (
	"time"

	libCrypto "github.com/LerianStudio/lib-commons/v2/commons/crypto"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
)

// MongoDBModel represents the MongoDB document structure for a holder.
type MongoDBModel struct {
	ID            *uuid.UUID                 `bson:"_id,omitempty"`
	ExternalID    *string                    `bson:"external_id,omitempty"`
	Type          *string                    `bson:"type,omitempty"`
	Name          *string                    `bson:"name,omitempty"`
	Document      *string                    `bson:"document,omitempty"`
	Addresses     *AddressesMongoDBModel     `bson:"addresses,omitempty"`
	Contact       *ContactMongoDBModel       `bson:"contact,omitempty"`
	NaturalPerson *NaturalPersonMongoDBModel `bson:"natural_person,omitempty"`
	LegalPerson   *LegalPersonMongoDBModel   `bson:"legal_person,omitempty"`
	Metadata      map[string]any             `bson:"metadata"`
	Search        map[string]string          `bson:"search,omitempty"`
	CreatedAt     *time.Time                 `bson:"created_at,omitempty"`
	UpdatedAt     *time.Time                 `bson:"updated_at"`
	DeletedAt     *time.Time                 `bson:"deleted_at"`
}

// AddressesMongoDBModel represents the collection of addresses in MongoDB.
type AddressesMongoDBModel struct {
	Primary     *AddressMongoDBModel `bson:"primary,omitempty"`
	Additional1 *AddressMongoDBModel `bson:"additional_1,omitempty"`
	Additional2 *AddressMongoDBModel `bson:"additional_2,omitempty"`
}

// AddressMongoDBModel represents a single address subdocument in MongoDB.
type AddressMongoDBModel struct {
	Line1     *string `bson:"line_1,omitempty"`
	Line2     *string `bson:"line_2,omitempty"`
	ZipCode   *string `bson:"zip_code,omitempty"`
	City      *string `bson:"city,omitempty"`
	State     *string `bson:"state,omitempty"`
	Country   *string `bson:"country,omitempty"`
	IsPrimary *bool   `bson:"is_primary,omitempty"`
}

// ContactMongoDBModel represents contact information subdocument in MongoDB.
type ContactMongoDBModel struct {
	PrimaryEmail   *string `bson:"primary_email,omitempty"`
	SecondaryEmail *string `bson:"secondary_email,omitempty"`
	MobilePhone    *string `bson:"mobile_phone,omitempty"`
	OtherPhone     *string `bson:"other_phone,omitempty"`
}

// NaturalPersonMongoDBModel represents natural person details subdocument in MongoDB.
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

// LegalPersonMongoDBModel represents legal person details subdocument in MongoDB.
type LegalPersonMongoDBModel struct {
	TradeName      *string                     `bson:"trade_name,omitempty"`
	Activity       *string                     `bson:"activity,omitempty"`
	Type           *string                     `bson:"type,omitempty"`
	FoundingDate   *time.Time                  `bson:"founding_date,omitempty"`
	Size           *string                     `bson:"size,omitempty"`
	Status         *string                     `bson:"status,omitempty"`
	Representative *RepresentativeMongoDBModel `bson:"representative,omitempty"`
}

// RepresentativeMongoDBModel represents a legal person representative subdocument in MongoDB.
type RepresentativeMongoDBModel struct {
	Name     *string `bson:"name,omitempty"`
	Document *string `bson:"document,omitempty"`
	Email    *string `bson:"email,omitempty"`
	Role     *string `bson:"role,omitempty"`
}

// FromEntity maps a holder entity to a MongoDB Holder model
func (hmm *MongoDBModel) FromEntity(h *mmodel.Holder, ds *libCrypto.Crypto) error {
	name, err := ds.Encrypt(h.Name)
	if err != nil {
		return pkg.ValidateInternalError(err, "Holder")
	}

	document, err := ds.Encrypt(h.Document)
	if err != nil {
		return pkg.ValidateInternalError(err, "Holder")
	}

	*hmm = MongoDBModel{
		ID:         h.ID,
		ExternalID: h.ExternalID,
		Type:       h.Type,
		Name:       name,
		Document:   document,
		CreatedAt:  &h.CreatedAt,
		UpdatedAt:  &h.UpdatedAt,
		DeletedAt:  h.DeletedAt,
	}

	if err := hmm.setRelatedEntities(h, ds); err != nil {
		return err
	}

	hmm.setSearchAndMetadata(h, ds)

	return nil
}

func (hmm *MongoDBModel) setRelatedEntities(h *mmodel.Holder, ds *libCrypto.Crypto) error {
	if h.Addresses != nil {
		hmm.Addresses = mapAddressesFromEntity(h.Addresses)
	}

	if h.Contact != nil {
		contact, err := mapContactFromEntity(ds, h.Contact)
		if err != nil {
			return err
		}

		hmm.Contact = contact
	}

	if h.NaturalPerson != nil {
		naturalPerson, err := mapNaturalPersonFromEntity(ds, h.NaturalPerson)
		if err != nil {
			return err
		}

		hmm.NaturalPerson = naturalPerson
	}

	if h.LegalPerson != nil {
		legalPerson, err := mapLegalPersonFromEntity(ds, h.LegalPerson)
		if err != nil {
			return err
		}

		hmm.LegalPerson = legalPerson
	}

	return nil
}

func (hmm *MongoDBModel) setSearchAndMetadata(h *mmodel.Holder, ds *libCrypto.Crypto) {
	hmm.Search = make(map[string]string)
	if h.Document != nil && *h.Document != "" {
		hmm.Search["document"] = ds.GenerateHash(h.Document)
	}

	if h.Metadata == nil {
		hmm.Metadata = make(map[string]any)
	} else {
		hmm.Metadata = h.Metadata
	}
}

// mapAddressesFromEntity maps addresses entity to MongoDB model
func mapAddressesFromEntity(a *mmodel.Addresses) *AddressesMongoDBModel {
	return &AddressesMongoDBModel{
		Primary:     mapAddressFromEntity(a.Primary),
		Additional1: mapAddressFromEntity(a.Additional1),
		Additional2: mapAddressFromEntity(a.Additional2),
	}
}

// mapContactFromEntity maps contact entity to MongoDB model
func mapContactFromEntity(ds *libCrypto.Crypto, c *mmodel.Contact) (*ContactMongoDBModel, error) {
	primaryEmail, err := ds.Encrypt(c.PrimaryEmail)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	secondaryEmail, err := ds.Encrypt(c.SecondaryEmail)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	mobilePhone, err := ds.Encrypt(c.MobilePhone)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	otherPhone, err := ds.Encrypt(c.OtherPhone)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	return &ContactMongoDBModel{
		PrimaryEmail:   primaryEmail,
		SecondaryEmail: secondaryEmail,
		MobilePhone:    mobilePhone,
		OtherPhone:     otherPhone,
	}, nil
}

// mapNaturalPersonFromEntity maps natural person entity to MongoDB model
func mapNaturalPersonFromEntity(ds *libCrypto.Crypto, np *mmodel.NaturalPerson) (*NaturalPersonMongoDBModel, error) {
	motherName, err := ds.Encrypt(np.MotherName)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	fatherName, err := ds.Encrypt(np.FatherName)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	return &NaturalPersonMongoDBModel{
		FavoriteName: np.FavoriteName,
		SocialName:   np.SocialName,
		Gender:       np.Gender,
		BirthDate:    np.BirthDate,
		CivilStatus:  np.CivilStatus,
		Nationality:  np.Nationality,
		MotherName:   motherName,
		FatherName:   fatherName,
		Status:       np.Status,
	}, nil
}

// mapLegalPersonFromEntity maps legal person entity to MongoDB model
func mapLegalPersonFromEntity(ds *libCrypto.Crypto, lp *mmodel.LegalPerson) (*LegalPersonMongoDBModel, error) {
	var parsedFoundingDate *time.Time

	if lp.FoundingDate != nil {
		parsed, err := time.Parse("2006-01-02", *lp.FoundingDate)
		if err != nil {
			return nil, pkg.ValidateInternalError(err, "Holder")
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
		repName, err := ds.Encrypt(lp.Representative.Name)
		if err != nil {
			return nil, pkg.ValidateInternalError(err, "Holder")
		}

		repDocument, err := ds.Encrypt(lp.Representative.Document)
		if err != nil {
			return nil, pkg.ValidateInternalError(err, "Holder")
		}

		repEmail, err := ds.Encrypt(lp.Representative.Email)
		if err != nil {
			return nil, pkg.ValidateInternalError(err, "Holder")
		}

		mongoLP.Representative = &RepresentativeMongoDBModel{
			Name:     repName,
			Document: repDocument,
			Email:    repEmail,
			Role:     lp.Representative.Role,
		}
	}

	return mongoLP, nil
}

// mapAddressFromEntity maps an address entity to MongoDB model
func mapAddressFromEntity(a *mmodel.Address) *AddressMongoDBModel {
	if a == nil {
		return nil
	}

	return &AddressMongoDBModel{
		Line1:   &a.Line1,
		Line2:   a.Line2,
		ZipCode: &a.ZipCode,
		City:    &a.City,
		State:   &a.State,
		Country: &a.Country,
	}
}

// ToEntity maps a MongoDB model to a Holder entity
func (hmm *MongoDBModel) ToEntity(ds *libCrypto.Crypto) (*mmodel.Holder, error) {
	name, err := ds.Decrypt(hmm.Name)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	document, err := ds.Decrypt(hmm.Document)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	holder := &mmodel.Holder{
		ID:         hmm.ID,
		ExternalID: hmm.ExternalID,
		Type:       hmm.Type,
		Name:       name,
		Document:   document,
		Metadata:   hmm.Metadata,
		CreatedAt:  utils.SafeTimePtr(hmm.CreatedAt),
		UpdatedAt:  utils.SafeTimePtr(hmm.UpdatedAt),
		DeletedAt:  hmm.DeletedAt,
	}

	if hmm.Addresses != nil {
		holder.Addresses = mapAddressesToEntity(hmm.Addresses)
	}

	if hmm.Contact != nil {
		contact, err := mapContactToEntity(ds, hmm.Contact)
		if err != nil {
			return nil, err
		}

		holder.Contact = contact
	}

	if hmm.NaturalPerson != nil {
		np, err := mapNaturalPersonToEntity(ds, hmm.NaturalPerson)
		if err != nil {
			return nil, err
		}

		holder.NaturalPerson = np
	}

	if hmm.LegalPerson != nil {
		lp, err := mapLegalPersonToEntity(ds, hmm.LegalPerson)
		if err != nil {
			return nil, err
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

// mapContactToEntity maps a MongoDB model to a Contact entity
func mapContactToEntity(ds *libCrypto.Crypto, c *ContactMongoDBModel) (*mmodel.Contact, error) {
	primaryEmail, err := ds.Decrypt(c.PrimaryEmail)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	secondaryEmail, err := ds.Decrypt(c.SecondaryEmail)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	mobilePhone, err := ds.Decrypt(c.MobilePhone)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	otherPhone, err := ds.Decrypt(c.OtherPhone)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	return &mmodel.Contact{
		PrimaryEmail:   primaryEmail,
		SecondaryEmail: secondaryEmail,
		MobilePhone:    mobilePhone,
		OtherPhone:     otherPhone,
	}, nil
}

// mapNaturalPersonToEntity maps a MongoDB model to a NaturalPerson entity
func mapNaturalPersonToEntity(ds *libCrypto.Crypto, np *NaturalPersonMongoDBModel) (*mmodel.NaturalPerson, error) {
	motherName, err := ds.Decrypt(np.MotherName)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	fatherName, err := ds.Decrypt(np.FatherName)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	return &mmodel.NaturalPerson{
		FavoriteName: np.FavoriteName,
		SocialName:   np.SocialName,
		Gender:       np.Gender,
		BirthDate:    np.BirthDate,
		CivilStatus:  np.CivilStatus,
		Nationality:  np.Nationality,
		MotherName:   motherName,
		FatherName:   fatherName,
		Status:       np.Status,
	}, nil
}

// mapLegalPersonToEntity maps a MongoDB model to a LegalPerson entity
func mapLegalPersonToEntity(ds *libCrypto.Crypto, lp *LegalPersonMongoDBModel) (*mmodel.LegalPerson, error) {
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
		rep, err := mapRepresentativeToEntity(ds, lp.Representative)
		if err != nil {
			return nil, err
		}

		legalPerson.Representative = rep
	}

	return legalPerson, nil
}

// mapRepresentativeToEntity maps a MongoDB model to a Representative entity
func mapRepresentativeToEntity(ds *libCrypto.Crypto, rep *RepresentativeMongoDBModel) (*mmodel.Representative, error) {
	representativeName, err := ds.Decrypt(rep.Name)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	representativeDocument, err := ds.Decrypt(rep.Document)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	email, err := ds.Decrypt(rep.Email)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Holder")
	}

	return &mmodel.Representative{
		Name:     representativeName,
		Document: representativeDocument,
		Email:    email,
		Role:     rep.Role,
	}, nil
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
		Line1:   line1,
		Line2:   a.Line2,
		ZipCode: zipCode,
		City:    city,
		State:   state,
		Country: country,
	}
}
