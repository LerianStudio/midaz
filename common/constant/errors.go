package constant

import (
	"errors"
	"fmt"
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/net/http"
)

var (
	DuplicateLedgerBusinessError                     = errors.New("0001")
	LedgerNameConflictBusinessError                  = errors.New("0002")
	AssetNameOrCodeDuplicateBusinessError            = errors.New("0003")
	CodeUppercaseRequirementBusinessError            = errors.New("0004")
	CurrencyCodeStandardComplianceBusinessError      = errors.New("0005")
	UnmodifiableFieldBusinessError                   = errors.New("0006")
	EntityNotFoundBusinessError                      = errors.New("0007")
	ActionNotPermittedBusinessError                  = errors.New("0008")
	MissingFieldsInRequestBusinessError              = errors.New("0009")
	AccountTypeImmutableBusinessError                = errors.New("0010")
	InactiveAccountTypeBusinessError                 = errors.New("0011")
	AccountBalanceDeletionBusinessError              = errors.New("0012")
	ResourceAlreadyDeletedBusinessError              = errors.New("0013")
	ProductIDInactiveBusinessError                   = errors.New("0014")
	DuplicateProductNameBusinessError                = errors.New("0015")
	BalanceRemainingDeletionBusinessError            = errors.New("0016")
	InvalidScriptFormatBusinessError                 = errors.New("0017")
	InsufficientFundsBusinessError                   = errors.New("0018")
	AccountIneligibilityBusinessError                = errors.New("0019")
	AliasUnavailabilityBusinessError                 = errors.New("0020")
	ParentTransactionIDNotFoundBusinessError         = errors.New("0021")
	ImmutableFieldBusinessError                      = errors.New("0022")
	TransactionTimingRestrictionBusinessError        = errors.New("0023")
	AccountStatusTransactionRestrictionBusinessError = errors.New("0024")
	InsufficientAccountBalanceBusinessError          = errors.New("0025")
	TransactionMethodRestrictionBusinessError        = errors.New("0026")
	DuplicateTransactionTemplateCodeBusinessError    = errors.New("0027")
	DuplicateAssetPairBusinessError                  = errors.New("0028")
	InvalidParentAccountIDBusinessError              = errors.New("0029")
	MismatchedAssetCodeBusinessError                 = errors.New("0030")
	ChartTypeNotFoundBusinessError                   = errors.New("0031")
	InvalidCountryCodeBusinessError                  = errors.New("0032")
	InvalidCodeFormatBusinessError                   = errors.New("0033")
	AssetCodeNotFoundBusinessError                   = errors.New("0034")
	PortfolioIDNotFoundBusinessError                 = errors.New("0035")
	ProductIDNotFoundBusinessError                   = errors.New("0036")
	LedgerIDNotFoundBusinessError                    = errors.New("0037")
	OrganizationIDNotFoundBusinessError              = errors.New("0038")
	ParentOrganizationIDNotFoundBusinessError        = errors.New("0039")
	InvalidTypeBusinessError                         = errors.New("0040")
	TokenMissingBusinessError                        = errors.New("0041")
	InvalidTokenBusinessError                        = errors.New("0042")
	InsufficientPrivilegesBusinessError              = errors.New("0043")
	PermissionEnforcementBusinessError               = errors.New("0044")
	JWKFetchBusinessError                            = errors.New("0045")
	InternalServerBusinessError                      = errors.New("0046")
	BadRequestBusinessError                          = errors.New("0047")
	InvalidDSLFileFormatBusinessError                = errors.New("0048")
	EmptyDSLFileBusinessError                        = errors.New("0049")
	MetadataKeyLengthExceededBusinessError           = errors.New("0050")
	MetadataValueLengthExceededBusinessError         = errors.New("0051")
	AccountIDNotFoundBusinessError                   = errors.New("0052")
	UnexpectedFieldsInTheRequestBusinessError        = errors.New("0053")
	NoAccountsFoundBusinessError                     = errors.New("0054")
	AssetNotFoundBusinessError                       = errors.New("0055")
	NoAssetsFoundBusinessError                       = errors.New("0056")
	NoProductsFoundBusinessError                     = errors.New("0057")
	NoPortfoliosFoundBusinessError                   = errors.New("0058")
	NoOrganizationsFoundBusinessError                = errors.New("0059")
	NoLedgersFoundBusinessError                      = errors.New("0060")
	BalanceUpdateFailedBusinessError                 = errors.New("0061")
	NoAccountIDsProvidedBusinessError                = errors.New("0062")
	FailedToRetrieveAccountsByAliasesBusinessError   = errors.New("0063")
)

// ValidateInternalError validate the error and return the appropriate internal error code, title and message
func ValidateInternalError(err error, entityType string) error {
	return ValidateBusinessError(InternalServerBusinessError, entityType, err)
}

// ValidateBadRequestFieldsError validate the error and return the appropriate bad request error code, title, message and the invalid fields
func ValidateBadRequestFieldsError(knownInvalidFields map[string]string, entityType string, unknownFields map[string]any) error {
	if len(unknownFields) > 0 {
		return ValidateBusinessError(UnexpectedFieldsInTheRequestBusinessError, entityType, unknownFields)
	}
	return ValidateBusinessError(BadRequestBusinessError, entityType, knownInvalidFields)
}

// ValidateBusinessError validate the error and return the appropriate business error code, title and message
func ValidateBusinessError(err error, entityType string, args ...interface{}) error {
	switch {
	case errors.Is(err, AssetNameOrCodeDuplicateBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       "0003",
			Title:      "Asset Name or Code Duplicate",
			Message:    "An asset with the same name or code already exists in your ledger. Please modify the name or code of your new asset.",
		}
	case errors.Is(err, CodeUppercaseRequirementBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0004",
			Title:      "Code Uppercase Requirement",
			Message:    "The code must be in uppercase. Please ensure that the code is in uppercase format and try again.",
		}
	case errors.Is(err, CurrencyCodeStandardComplianceBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0005",
			Title:      "Currency Code Standard Compliance",
			Message:    "Currency-type assets must comply with the ISO-4217 standard. Please use a currency code that conforms to ISO-4217 guidelines.",
		}
	case errors.Is(err, EntityNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0007",
			Title:      "Entity Not Found",
			Message:    "No entity was found for the given ID. Please make sure to use the correct ID for the entity you are trying to manage.",
		}
	case errors.Is(err, ActionNotPermittedBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0008",
			Title:      "Action Not Permitted",
			Message:    "The action you are attempting is not allowed in the current environment. Please refer to the documentation for guidance.",
		}
	case errors.Is(err, DuplicateProductNameBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       "0015",
			Title:      "Duplicate Product Name Error",
			Message:    fmt.Sprintf("A product with the name %s already exists for this ledger ID %s. Please try again with a different ledger or name.", args...),
		}
	case errors.Is(err, AliasUnavailabilityBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       "0020",
			Title:      "Alias Unavailability Error",
			Message:    fmt.Sprintf("The alias %s is already in use. Please choose a different alias and try again.", args...),
		}
	case errors.Is(err, InvalidParentAccountIDBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0029",
			Title:      "Invalid Parent Account ID",
			Message:    "The specified parent account ID does not exist. Please verify the ID is correct and attempt your request again.",
		}
	case errors.Is(err, MismatchedAssetCodeBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0030",
			Title:      "Mismatched Asset Code",
			Message:    "The parent account ID you provided is associated with a different asset code than the one specified in your request. Please make sure the asset code matches that of the parent account, or use a different parent account ID and try again.",
		}
	case errors.Is(err, InvalidCountryCodeBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0032",
			Title:      "Invalid Country Code",
			Message:    "The provided country code in the 'address.country' field does not conform to the ISO-3166 alpha-2 standard. Please provide a valid alpha-2 country code.",
		}
	case errors.Is(err, AssetCodeNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0034",
			Title:      "Asset Code Not Found",
			Message:    "The provided asset code does not exist in our records. Please verify the asset code and try again.",
		}
	case errors.Is(err, PortfolioIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0035",
			Title:      "Portfolio ID Not Found",
			Message:    "The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again.",
		}
	case errors.Is(err, ProductIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0036",
			Title:      "Product ID Not Found",
			Message:    "The provided product ID does not exist in our records. Please verify the product ID and try again.",
		}
	case errors.Is(err, LedgerIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0037",
			Title:      "Ledger ID Not Found",
			Message:    "The provided ledger ID does not exist in our records. Please verify the ledger ID and try again.",
		}
	case errors.Is(err, OrganizationIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0038",
			Title:      "Organization ID Not Found",
			Message:    "The provided organization ID does not exist in our records. Please verify the organization ID and try again.",
		}
	case errors.Is(err, ParentOrganizationIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0039",
			Title:      "Parent Organization ID Not Found",
			Message:    "The provided parent organization ID does not exist in our records. Please verify the parent organization ID and try again.",
		}
	case errors.Is(err, InvalidTypeBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0040",
			Title:      "Invalid Type",
			Message:    "The provided 'type' is not valid. Accepted types are currency, crypto, commodities, or others. Please provide a valid type.",
		}
	case errors.Is(err, TokenMissingBusinessError):
		return common.UnauthorizedError{
			EntityType: entityType,
			Code:       "0041",
			Title:      "Token Missing",
			Message:    "A valid token must be provided in the request header. Please include a token and try again.",
		}
	case errors.Is(err, InvalidTokenBusinessError):
		return common.UnauthorizedError{
			EntityType: entityType,
			Code:       "0042",
			Title:      "Invalid Token",
			Message:    "The provided token is expired, invalid or malformed. Please provide a valid token and try again.",
		}
	case errors.Is(err, InsufficientPrivilegesBusinessError):
		return common.ForbiddenError{
			EntityType: entityType,
			Code:       "0043",
			Title:      "Insufficient Privileges",
			Message:    "You do not have the necessary permissions to perform this action. Please contact your administrator if you believe this is an error.",
		}
	case errors.Is(err, PermissionEnforcementBusinessError):
		return common.FailedPreconditionError{
			EntityType: entityType,
			Code:       "0044",
			Title:      "Permission Enforcement Error",
			Message:    "The enforcer is not configured properly. Please contact your administrator if you believe this is an error.",
		}
	case errors.Is(err, JWKFetchBusinessError):
		return common.FailedPreconditionError{
			EntityType: entityType,
			Code:       "0045",
			Title:      "JWK Fetch Error",
			Message:    "The JWK keys could not be fetched from the source. Please verify the source environment variable configuration and try again.",
		}
	case errors.Is(err, InternalServerBusinessError):
		e, ok := args[0].(error)
		if !ok {
			return fmt.Errorf("expected error type, got %T", args[0])
		}

		return common.InternalServerError{
			EntityType: entityType,
			Code:       "0046",
			Title:      "Internal Server Error",
			Message:    "The server encountered an unexpected error. Please try again later or contact support.",
			Err:        e,
		}
	case errors.Is(err, BadRequestBusinessError):
		fields, ok := args[0].(map[string]string)
		if !ok {
			return fmt.Errorf("expected knownInvalidFields of type map[string]string type, got %T", args[0])
		}

		return http.ValidationKnownFieldsError{
			EntityType: entityType,
			Code:       "0047",
			Title:      "Bad Request",
			Message:    "The server could not understand the request due to malformed syntax. Please check the listed fields and try again.",
			Fields:     fields,
		}
	case errors.Is(err, InvalidDSLFileFormatBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0048",
			Title:      "Invalid DSL File Format",
			Message:    fmt.Sprintf("The submitted DSL file %s is in an incorrect format. Please ensure that the file follows the expected structure and syntax.", args...),
		}
	case errors.Is(err, EmptyDSLFileBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0049",
			Title:      "Empty DSL File",
			Message:    fmt.Sprintf("The submitted DSL file %s is empty. Please provide a valid file with content.", args...),
		}
	case errors.Is(err, MetadataKeyLengthExceededBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0050",
			Title:      "Metadata Key Length Exceeded",
			Message:    fmt.Sprintf("The metadata key %s exceeds the maximum allowed length of 100 characters. Please use a shorter key.", args...),
		}
	case errors.Is(err, MetadataValueLengthExceededBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       "0051",
			Title:      "Metadata Value Length Exceeded",
			Message:    fmt.Sprintf("The metadata value %s exceeds the maximum allowed length of 100 characters. Please use a shorter value.", args...),
		}
	case errors.Is(err, AccountIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0052",
			Title:      "Account ID Not Found",
			Message:    "The provided account ID does not exist in our records. Please verify the account ID and try again.",
		}
	case errors.Is(err, UnexpectedFieldsInTheRequestBusinessError):
		fields, ok := args[0].(map[string]any)
		if !ok {
			return fmt.Errorf("expected unknownFields of type map[string]any type, got %T", args[0])
		}

		return http.ValidationUnknownFieldsError{
			EntityType: entityType,
			Code:       "0053",
			Title:      "Unexpected Fields in the Request",
			Message:    "The request body contains more fields than expected. Please send only the allowed fields as per the documentation. The unexpected fields are listed in the fields object.",
			Fields:     fields,
		}
	case errors.Is(err, NoAccountsFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0054",
			Title:      "Accounts Not Found for Provided IDs",
			Message:    "No accounts were found for the provided account IDs. Please verify the account IDs and try again.",
		}
	case errors.Is(err, AssetNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0055",
			Title:      "Asset Not Found",
			Message:    fmt.Sprintf("The specified asset ID %s was not found. Please verify the asset ID and try again.", args...),
		}
	case errors.Is(err, NoAssetsFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0056",
			Title:      "No Assets Found",
			Message:    "No assets were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, NoProductsFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0057",
			Title:      "No Products Found",
			Message:    "No products were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, NoPortfoliosFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0058",
			Title:      "No Portfolios Found",
			Message:    "No portfolios were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, NoOrganizationsFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0059",
			Title:      "No Organizations Found",
			Message:    "No organizations were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, NoLedgersFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0060",
			Title:      "No Ledgers Found",
			Message:    "No ledgers were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, BalanceUpdateFailedBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0061",
			Title:      "Balance Update Failed",
			Message:    "The balance could not be updated for the specified account ID. Please verify the account ID and try again.",
		}
	case errors.Is(err, NoAccountIDsProvidedBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0062",
			Title:      "No Account IDs Provided",
			Message:    "No account IDs were provided for the balance update. Please provide valid account IDs and try again.",
		}
	case errors.Is(err, FailedToRetrieveAccountsByAliasesBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       "0063",
			Title:      "Failed To Retrieve Accounts By Aliases",
			Message:    "The accounts could not be retrieved using the specified aliases. Please verify the aliases for accuracy and try again.",
		}
	default:
		return err
	}
}
