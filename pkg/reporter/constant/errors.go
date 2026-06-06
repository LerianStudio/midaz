// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import (
	"errors"
)

// List of errors that can be returned.
// You can standardize errors
// Standardized error
var (
	ErrMissingRequiredFields           = errors.New("TPL-0001")
	ErrInvalidFileFormat               = errors.New("TPL-0002")
	ErrInvalidOutputFormat             = errors.New("TPL-0003")
	ErrInvalidHeaderParameter          = errors.New("TPL-0004")
	ErrInvalidFileUploaded             = errors.New("TPL-0005")
	ErrEmptyFile                       = errors.New("TPL-0006")
	ErrFileContentInvalid              = errors.New("TPL-0007")
	ErrInvalidMapFields                = errors.New("TPL-0008")
	ErrInvalidPathParameter            = errors.New("TPL-0009")
	ErrOutputFormatWithoutTemplateFile = errors.New("TPL-0010")
	ErrEntityNotFound                  = errors.New("TPL-0011")
	ErrInvalidTemplateID               = errors.New("TPL-0012")
	ErrInvalidLedgerIDList             = errors.New("TPL-0013")
	ErrMissingTableFields              = errors.New("TPL-0014")
	ErrUnexpectedFieldsInTheRequest    = errors.New("TPL-0015")
	ErrMissingFieldsInRequest          = errors.New("TPL-0016")
	ErrBadRequest                      = errors.New("TPL-0017")
	ErrInternalServer                  = errors.New("TPL-0018")
	ErrInvalidQueryParameter           = errors.New("TPL-0019")
	ErrInvalidDateFormat               = errors.New("TPL-0020")
	ErrInvalidFinalDate                = errors.New("TPL-0021")
	ErrDateRangeExceedsLimit           = errors.New("TPL-0022")
	ErrInvalidDateRange                = errors.New("TPL-0023")
	ErrPaginationLimitExceeded         = errors.New("TPL-0024")
	ErrInvalidSortOrder                = errors.New("TPL-0025")
	ErrMetadataKeyLengthExceeded       = errors.New("TPL-0026")
	ErrMetadataValueLengthExceeded     = errors.New("TPL-0027")
	ErrInvalidMetadataNesting          = errors.New("TPL-0028")
	ErrReportStatusNotFinished         = errors.New("TPL-0029")
	ErrMissingSchemaTable              = errors.New("TPL-0030")
	ErrMissingDataSource               = errors.New("TPL-0031")
	ErrScriptTagDetected               = errors.New("TPL-0032")
	ErrDecryptionData                  = errors.New("TPL-0033")
	ErrCommunicateSeaweedFS            = errors.New("TPL-0034")
	ErrSchemaAmbiguous                 = errors.New("TPL-0035")
	ErrSchemaNotFound                  = errors.New("TPL-0036")
	ErrTableNotFoundInSchema           = errors.New("TPL-0037")
	ErrDatabaseNotRegistered           = errors.New("TPL-0038")
	ErrDuplicateRequestInFlight        = errors.New("TPL-0039")
	ErrIdempotencyConflict             = errors.New("TPL-0040")
	ErrBucketRequired                  = errors.New("TPL-0041")
	ErrObjectKeyRequired               = errors.New("TPL-0042")
	ErrObjectNotFound                  = errors.New("TPL-0043")
	ErrTTLNotSupported                 = errors.New("TPL-0044")
	ErrDuplicateDeadline               = errors.New("TPL-0045")
	ErrInvalidDeadlineType             = errors.New("TPL-0046")
	ErrInvalidDeadlineFrequency        = errors.New("TPL-0047")
	ErrInvalidDeadlineColor            = errors.New("TPL-0048")
	ErrMonthsOfYearNotApplicable       = errors.New("TPL-0050")
	ErrMonthsOfYearRequired            = errors.New("TPL-0052")
	ErrMonthsOfYearOutOfRange          = errors.New("TPL-0054")
	ErrDueDateInPast                   = errors.New("TPL-0055")
	ErrMonthsOfYearCountMismatch       = errors.New("TPL-0056")
	ErrDataSourceNotFound              = errors.New("TPL-0057")
	ErrDataSourceUnavailable           = errors.New("TPL-0058")
	ErrSchemaValidationFailed          = errors.New("TPL-0059")
	ErrExtractionJobFailed             = errors.New("TPL-0060")
	ErrInvalidUTF8                     = errors.New("TPL-0061")
	ErrTemplateRenderFailed            = errors.New("TPL-0062")
)

// REP- codes identify worker-internal pipeline errors. They are carried by
// typed errors (ValidationError, FailedPreconditionError) constructed at the
// failure site with dynamic context, not routed through ValidateBusinessError.
// Every REP- code must be declared here so new codes cannot collide.
// Gaps: REP-0071 and REP-0081 are retired; do not reuse.
const (
	ErrCodeDataSourceNotFound         = "REP-0060"
	ErrCodeDataSourceUnavailable      = "REP-0061"
	ErrCodeUnsupportedDatabaseType    = "REP-0062"
	ErrCodeUnexpectedSchemaResult     = "REP-0063"
	ErrCodeUnexpectedTableResult      = "REP-0064"
	ErrCodeUnexpectedCollectionResult = "REP-0065"
	ErrCodeCRMHashKeyNotConfigured    = "REP-0066"
	ErrCodeCRMEncryptKeyNotConfigured = "REP-0067"
	ErrCodeCipherInitFailed           = "REP-0068"
	ErrCodeRecordDecryptionFailed     = "REP-0069"
	ErrCodeStorageNotConfigured       = "REP-0070"
	ErrCodeInvalidExtractedData       = "REP-0072"
	ErrCodeEmptyEncryptedData         = "REP-0073"
	ErrCodeDecryptionKeyNotConfigured = "REP-0074"
	ErrCodeInvalidEncryptedData       = "REP-0075"
	ErrCodeAESCipherCreationFailed    = "REP-0076"
	ErrCodeGCMCreationFailed          = "REP-0077"
	ErrCodeCorruptEncryptedData       = "REP-0078"
	ErrCodeAESGCMDecryptionFailed     = "REP-0079"
	ErrCodeInvalidFetcherResponse     = "REP-0080"
)
