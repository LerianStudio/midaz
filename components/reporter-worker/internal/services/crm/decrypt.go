// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package crm

import (
	"fmt"
	"strings"

	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	"github.com/LerianStudio/lib-observability/log"

	pkgErr "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
)

// DecryptRecords decrypts the crm sensitive fields on each extracted record in
// place and returns the slice. It first decides whether any requested field
// implies decryption (a known top-level encrypted field, or any nested/dotted
// field), short-circuiting to the input untouched when none do, then initializes
// the AES-GCM cipher and walks each record's encrypted-field tree.
//
// Both keys are required when decryption is needed; an empty key fails closed
// with a precondition error. The keys are supplied by the caller
// (UseCase.CryptoHashSecretKeyCRM / UseCase.CryptoEncryptSecretKeyCRM); this
// function neither holds nor logs them, and it never logs decrypted values.
func DecryptRecords(records []map[string]any, fields []string, hashKey, encryptKey string, logger log.Logger) ([]map[string]any, error) {
	if !needsDecryption(fields) {
		return records, nil
	}

	if encryptKey == "" {
		return nil, pkgErr.FailedPreconditionError{
			Code:    cnErr.ErrCodeCRMEncryptKeyNotConfigured.Error(),
			Title:   "CRM Crypto Not Configured",
			Message: "CRYPTO_ENCRYPT_SECRET_KEY_CRM not configured",
		}
	}

	if hashKey == "" {
		return nil, pkgErr.FailedPreconditionError{
			Code:    cnErr.ErrCodeCRMHashKeyNotConfigured.Error(),
			Title:   "CRM Crypto Not Configured",
			Message: "CRYPTO_HASH_SECRET_KEY_CRM not configured",
		}
	}

	crypto := &libCrypto.Crypto{
		HashSecretKey:    hashKey,
		EncryptSecretKey: encryptKey,
		Logger:           logger,
	}

	if err := crypto.InitializeCipher(); err != nil {
		return nil, pkgErr.FailedPreconditionError{
			Code:    cnErr.ErrCodeCipherInitFailed.Error(),
			Title:   "Cipher Initialization Failed",
			Message: fmt.Sprintf("failed to initialize cipher: %s", err.Error()),
			Err:     err,
		}
	}

	for i, record := range records {
		decryptedRecord, err := decryptRecord(record, crypto)
		if err != nil {
			return nil, pkgErr.FailedPreconditionError{
				Code:    cnErr.ErrCodeRecordDecryptionFailed.Error(),
				Title:   "Decryption Failed",
				Message: fmt.Sprintf("failed to decrypt record %d: %s", i, err.Error()),
				Err:     err,
			}
		}

		records[i] = decryptedRecord
	}

	return records, nil
}

// needsDecryption reports whether any requested field implies decryption work,
// mirroring the legacy decision: a known top-level encrypted field, or any
// dotted/nested field reference (which may touch a nested encrypted object).
func needsDecryption(fields []string) bool {
	for _, field := range fields {
		if isEncryptedField(field) {
			return true
		}

		if strings.Contains(field, ".") {
			return true
		}
	}

	return false
}

// isEncryptedField reports whether a top-level field is known to be encrypted in
// crm.
func isEncryptedField(field string) bool {
	encryptedFields := map[string]bool{
		"document": true,
		"name":     true,
	}

	return encryptedFields[field]
}

// decryptRecord returns a shallow copy of the record with its encrypted
// top-level and nested fields decrypted, leaving the input untouched.
func decryptRecord(record map[string]any, crypto *libCrypto.Crypto) (map[string]any, error) {
	decryptedRecord := make(map[string]any, len(record))
	for k, v := range record {
		decryptedRecord[k] = v
	}

	if err := decryptTopLevelFields(decryptedRecord, crypto); err != nil {
		return nil, err
	}

	if err := decryptNestedFields(decryptedRecord, crypto); err != nil {
		return nil, err
	}

	return decryptedRecord, nil
}

// decryptTopLevelFields decrypts the known top-level encrypted fields present on
// the record.
func decryptTopLevelFields(record map[string]any, crypto *libCrypto.Crypto) error {
	for fieldName, fieldValue := range record {
		if isEncryptedField(fieldName) && fieldValue != nil {
			if err := decryptFieldValue(record, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt field %s: %w", fieldName, err)
			}
		}
	}

	return nil
}

// decryptNestedFields decrypts every nested encrypted object the legacy worker
// handled, in the same order.
func decryptNestedFields(record map[string]any, crypto *libCrypto.Crypto) error {
	if err := decryptContactFields(record, crypto); err != nil {
		return err
	}

	if err := decryptBankingDetailsFields(record, crypto); err != nil {
		return err
	}

	if err := decryptLegalPersonFields(record, crypto); err != nil {
		return err
	}

	if err := decryptNaturalPersonFields(record, crypto); err != nil {
		return err
	}

	if err := decryptRegulatoryFieldsFields(record, crypto); err != nil {
		return err
	}

	return decryptRelatedPartiesFields(record, crypto)
}

// decryptContactFields decrypts the encrypted fields within the contact object.
func decryptContactFields(record map[string]any, crypto *libCrypto.Crypto) error {
	contact, ok := record["contact"].(map[string]any)
	if !ok {
		return nil
	}

	for _, fieldName := range []string{"primary_email", "secondary_email", "mobile_phone", "other_phone"} {
		if fieldValue, exists := contact[fieldName]; exists && fieldValue != nil {
			if err := decryptFieldValue(contact, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt contact.%s: %w", fieldName, err)
			}
		}
	}

	record["contact"] = contact

	return nil
}

// decryptBankingDetailsFields decrypts the encrypted fields within the
// banking_details object.
func decryptBankingDetailsFields(record map[string]any, crypto *libCrypto.Crypto) error {
	bankingDetails, ok := record["banking_details"].(map[string]any)
	if !ok {
		return nil
	}

	for _, fieldName := range []string{"account", "iban"} {
		if fieldValue, exists := bankingDetails[fieldName]; exists && fieldValue != nil {
			if err := decryptFieldValue(bankingDetails, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt banking_details.%s: %w", fieldName, err)
			}
		}
	}

	record["banking_details"] = bankingDetails

	return nil
}

// decryptLegalPersonFields decrypts the encrypted fields within the
// legal_person.representative object.
func decryptLegalPersonFields(record map[string]any, crypto *libCrypto.Crypto) error {
	legalPerson, ok := record["legal_person"].(map[string]any)
	if !ok {
		return nil
	}

	representative, ok := legalPerson["representative"].(map[string]any)
	if !ok {
		return nil
	}

	for _, fieldName := range []string{"name", "document", "email"} {
		if fieldValue, exists := representative[fieldName]; exists && fieldValue != nil {
			if err := decryptFieldValue(representative, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt legal_person.representative.%s: %w", fieldName, err)
			}
		}
	}

	legalPerson["representative"] = representative
	record["legal_person"] = legalPerson

	return nil
}

// decryptNaturalPersonFields decrypts the encrypted fields within the
// natural_person object.
func decryptNaturalPersonFields(record map[string]any, crypto *libCrypto.Crypto) error {
	naturalPerson, ok := record["natural_person"].(map[string]any)
	if !ok {
		return nil
	}

	for _, fieldName := range []string{"mother_name", "father_name"} {
		if fieldValue, exists := naturalPerson[fieldName]; exists && fieldValue != nil {
			if err := decryptFieldValue(naturalPerson, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt natural_person.%s: %w", fieldName, err)
			}
		}
	}

	record["natural_person"] = naturalPerson

	return nil
}

// decryptRegulatoryFieldsFields decrypts the encrypted fields within the
// regulatory_fields object.
func decryptRegulatoryFieldsFields(record map[string]any, crypto *libCrypto.Crypto) error {
	regulatoryFields, ok := record["regulatory_fields"].(map[string]any)
	if !ok {
		return nil
	}

	for _, fieldName := range []string{"participant_document"} {
		if fieldValue, exists := regulatoryFields[fieldName]; exists && fieldValue != nil {
			if err := decryptFieldValue(regulatoryFields, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt regulatory_fields.%s: %w", fieldName, err)
			}
		}
	}

	record["regulatory_fields"] = regulatoryFields

	return nil
}

// decryptRelatedPartiesFields decrypts the document field within each
// related_parties array item.
func decryptRelatedPartiesFields(record map[string]any, crypto *libCrypto.Crypto) error {
	relatedParties, ok := record["related_parties"].([]any)
	if !ok {
		return nil
	}

	for i, party := range relatedParties {
		partyMap, ok := party.(map[string]any)
		if !ok {
			continue
		}

		if fieldValue, exists := partyMap["document"]; exists && fieldValue != nil {
			if err := decryptFieldValue(partyMap, "document", fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt related_parties[%d].document: %w", i, err)
			}
		}

		relatedParties[i] = partyMap
	}

	record["related_parties"] = relatedParties

	return nil
}

// decryptFieldValue decrypts a single non-empty string field in place. A
// non-string or empty value is left untouched.
func decryptFieldValue(container map[string]any, fieldName string, fieldValue any, crypto *libCrypto.Crypto) error {
	strValue, ok := fieldValue.(string)
	if !ok || strValue == "" {
		return nil
	}

	decryptedValue, err := crypto.Decrypt(&strValue)
	if err != nil {
		return err
	}

	container[fieldName] = *decryptedValue

	return nil
}
