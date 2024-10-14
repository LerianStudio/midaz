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
	IDsNotFoundForAccountsBusinessError              = errors.New("0054")
	AssetIDNotFoundBusinessError                     = errors.New("0055")
	NoAssetsFoundBusinessError                       = errors.New("0056")
	NoProductsFoundBusinessError                     = errors.New("0057")
	NoPortfoliosFoundBusinessError                   = errors.New("0058")
	NoOrganizationsFoundBusinessError                = errors.New("0059")
	NoLedgersFoundBusinessError                      = errors.New("0060")
	BalanceUpdateFailedBusinessError                 = errors.New("0061")
	NoAccountIDsProvidedBusinessError                = errors.New("0062")
	FailedToRetrieveAccountsByAliasesBusinessError   = errors.New("0063")
	NoAccountsFoundBusinessError                     = errors.New("0064")
)

// ValidateInternalError validate the error and return the appropriate internal error code, title and message
func ValidateInternalError(err error, entityType string) error {
	return common.InternalServerError{
		EntityType: entityType,
		Code:       InternalServerBusinessError.Error(),
		Title:      "Internal Server Error",
		Message:    "The server encountered an unexpected error. Please try again later or contact support.",
		Err:        err,
	}
}

// ValidateBadRequestFieldsError validate the error and return the appropriate bad request error code, title, message and the invalid fields
func ValidateBadRequestFieldsError(knownInvalidFields map[string]string, entityType string, unknownFields map[string]any) error {
	if len(unknownFields) == 0 && len(knownInvalidFields) == 0 {
		return errors.New("expected knownInvalidFields and unknownFields to be non-empty")
	}

	if len(unknownFields) > 0 {
		return http.ValidationUnknownFieldsError{
			EntityType: entityType,
			Code:       UnexpectedFieldsInTheRequestBusinessError.Error(),
			Title:      "Unexpected Fields in the Request",
			Message:    "The request body contains more fields than expected. Please send only the allowed fields as per the documentation. The unexpected fields are listed in the fields object.",
			Fields:     unknownFields,
		}
	}

	return http.ValidationKnownFieldsError{
		EntityType: entityType,
		Code:       BadRequestBusinessError.Error(),
		Title:      "Bad Request",
		Message:    "The server could not understand the request due to malformed syntax. Please check the listed fields and try again.",
		Fields:     knownInvalidFields,
	}
}

// ValidateBusinessError validate the error and return the appropriate business error code, title and message
func ValidateBusinessError(err error, entityType string, args ...interface{}) error {
	switch {
	case errors.Is(err, DuplicateLedgerBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       DuplicateLedgerBusinessError.Error(),
			Title:      "Duplicate Ledger Error",
			Message:    fmt.Sprintf("A ledger with the name %s already exists in the division %s. Please rename the ledger or choose a different division to attach it to.", args...),
		}
	case errors.Is(err, LedgerNameConflictBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       LedgerNameConflictBusinessError.Error(),
			Title:      "Ledger Name Conflict",
			Message:    fmt.Sprintf("A ledger named %s already exists in your organization. Please rename the ledger, or if you want to use the same name, consider creating a new ledger for a different division.", args...),
		}
	case errors.Is(err, AssetNameOrCodeDuplicateBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       AssetNameOrCodeDuplicateBusinessError.Error(),
			Title:      "Asset Name or Code Duplicate",
			Message:    "An asset with the same name or code already exists in your ledger. Please modify the name or code of your new asset.",
		}
	case errors.Is(err, CodeUppercaseRequirementBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       CodeUppercaseRequirementBusinessError.Error(),
			Title:      "Code Uppercase Requirement",
			Message:    "The code must be in uppercase. Please ensure that the code is in uppercase format and try again.",
		}
	case errors.Is(err, CurrencyCodeStandardComplianceBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       CurrencyCodeStandardComplianceBusinessError.Error(),
			Title:      "Currency Code Standard Compliance",
			Message:    "Currency-type assets must comply with the ISO-4217 standard. Please use a currency code that conforms to ISO-4217 guidelines.",
		}
	case errors.Is(err, UnmodifiableFieldBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       UnmodifiableFieldBusinessError.Error(),
			Title:      "Unmodifiable Field Error",
			Message:    "Your request includes a field that cannot be modified. Please review your request and try again, removing any uneditable fields. Please refer to the documentation for guidance.",
		}
	case errors.Is(err, EntityNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       EntityNotFoundBusinessError.Error(),
			Title:      "Entity Not Found",
			Message:    "No entity was found for the given ID. Please make sure to use the correct ID for the entity you are trying to manage.",
		}
	case errors.Is(err, ActionNotPermittedBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       ActionNotPermittedBusinessError.Error(),
			Title:      "Action Not Permitted",
			Message:    "The action you are attempting is not allowed in the current environment. Please refer to the documentation for guidance.",
		}
	case errors.Is(err, MissingFieldsInRequestBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       MissingFieldsInRequestBusinessError.Error(),
			Title:      "Missing Fields in Request",
			Message:    "Your request is missing one or more required fields. Please refer to the documentation to ensure all necessary fields are included in your request.",
		}
	case errors.Is(err, AccountTypeImmutableBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       AccountTypeImmutableBusinessError.Error(),
			Title:      "Account Type Immutable",
			Message:    "The account type specified cannot be modified. Please ensure the correct account type is being used and try again.",
		}
	case errors.Is(err, InactiveAccountTypeBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       InactiveAccountTypeBusinessError.Error(),
			Title:      "Inactive Account Type Error",
			Message:    "The account type specified cannot be set to INACTIVE. Please ensure the correct account type is being used and try again.",
		}
	case errors.Is(err, AccountBalanceDeletionBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       AccountBalanceDeletionBusinessError.Error(),
			Title:      "Account Balance Deletion Error",
			Message:    "An account or sub-account cannot be deleted if it has a remaining balance. Please ensure all remaining balances are transferred to another account before attempting to delete.",
		}
	case errors.Is(err, ResourceAlreadyDeletedBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       ResourceAlreadyDeletedBusinessError.Error(),
			Title:      "Resource Already Deleted",
			Message:    "The resource you are trying to delete has already been deleted. Ensure you are using the correct ID and try again.",
		}
	case errors.Is(err, ProductIDInactiveBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       ProductIDInactiveBusinessError.Error(),
			Title:      "Product ID Inactive",
			Message:    "The Product ID you are attempting to use is inactive. Please use another Product ID and try again.",
		}
	case errors.Is(err, DuplicateProductNameBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       DuplicateProductNameBusinessError.Error(),
			Title:      "Duplicate Product Name Error",
			Message:    fmt.Sprintf("A product with the name %s already exists for this ledger ID %s. Please try again with a different ledger or name.", args...),
		}
	case errors.Is(err, BalanceRemainingDeletionBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       BalanceRemainingDeletionBusinessError.Error(),
			Title:      "Balance Remaining Deletion Error",
			Message:    "The asset cannot be deleted because there is a remaining balance. Please ensure all balances are cleared before attempting to delete again.",
		}
	case errors.Is(err, InvalidScriptFormatBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       InvalidScriptFormatBusinessError.Error(),
			Title:      "Invalid Script Format Error",
			Message:    "The script provided in your request is invalid or in an unsupported format. Please verify the script format and try again.",
		}
	case errors.Is(err, InsufficientFundsBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       InsufficientFundsBusinessError.Error(),
			Title:      "Insufficient Funds Error",
			Message:    "The transaction could not be completed due to insufficient funds in the account. Please add sufficient funds to your account and try again.",
		}
	case errors.Is(err, AccountIneligibilityBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       AccountIneligibilityBusinessError.Error(),
			Title:      "Account Ineligibility Error",
			Message:    "One or more accounts listed in the transaction are not eligible to participate. Please review the account statuses and try again.",
		}
	case errors.Is(err, AliasUnavailabilityBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       AliasUnavailabilityBusinessError.Error(),
			Title:      "Alias Unavailability Error",
			Message:    fmt.Sprintf("The alias %s is already in use. Please choose a different alias and try again.", args...),
		}
	case errors.Is(err, ParentTransactionIDNotFoundBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       ParentTransactionIDNotFoundBusinessError.Error(),
			Title:      "Parent Transaction ID Not Found",
			Message:    fmt.Sprintf("The parentTransactionId %s does not correspond to any existing transaction. Please review the ID and try again.", args...),
		}
	case errors.Is(err, ImmutableFieldBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       ImmutableFieldBusinessError.Error(),
			Title:      "Immutable Field Error",
			Message:    fmt.Sprintf("The %s field cannot be modified. Please remove this field from your request and try again.", args...),
		}
	case errors.Is(err, TransactionTimingRestrictionBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       TransactionTimingRestrictionBusinessError.Error(),
			Title:      "Transaction Timing Restriction",
			Message:    fmt.Sprintf("You can only perform another transaction using %s of %f from %s to %s after %s. Please wait until the specified time to try again.", args...),
		}
	case errors.Is(err, AccountStatusTransactionRestrictionBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       AccountStatusTransactionRestrictionBusinessError.Error(),
			Title:      "Account Status Transaction Restriction",
			Message:    "The current statuses of the source and/or destination accounts do not permit transactions. Change the account status(es) and try again.",
		}
	case errors.Is(err, InsufficientAccountBalanceBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       InsufficientAccountBalanceBusinessError.Error(),
			Title:      "Insufficient Account Balance Error",
			Message:    fmt.Sprintf("The account %s does not have sufficient balance. Please try again with an amount that is less than or equal to the available balance.", args...),
		}
	case errors.Is(err, TransactionMethodRestrictionBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       TransactionMethodRestrictionBusinessError.Error(),
			Title:      "Transaction Method Restriction",
			Message:    fmt.Sprintf("Transactions involving %s are not permitted for the specified source and/or destination. Please try again using accounts that allow transactions with %s.", args...),
		}
	case errors.Is(err, DuplicateTransactionTemplateCodeBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       DuplicateTransactionTemplateCodeBusinessError.Error(),
			Title:      "Duplicate Transaction Template Code Error",
			Message:    fmt.Sprintf("A transaction template with the code %s already exists for your ledger. Please use a different code and try again.", args...),
		}
	case errors.Is(err, DuplicateAssetPairBusinessError):
		return common.EntityConflictError{
			EntityType: entityType,
			Code:       DuplicateAssetPairBusinessError.Error(),
			Title:      "Duplicate Asset Pair Error",
			Message:    fmt.Sprintf("A pair for the assets %s%s already exists with the ID %s. Please update the existing entry instead of creating a new one.", args...),
		}
	case errors.Is(err, InvalidParentAccountIDBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       InvalidParentAccountIDBusinessError.Error(),
			Title:      "Invalid Parent Account ID",
			Message:    "The specified parent account ID does not exist. Please verify the ID is correct and attempt your request again.",
		}
	case errors.Is(err, MismatchedAssetCodeBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       MismatchedAssetCodeBusinessError.Error(),
			Title:      "Mismatched Asset Code",
			Message:    "The parent account ID you provided is associated with a different asset code than the one specified in your request. Please make sure the asset code matches that of the parent account, or use a different parent account ID and try again.",
		}
	case errors.Is(err, ChartTypeNotFoundBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       ChartTypeNotFoundBusinessError.Error(),
			Title:      "Chart Type Not Found",
			Message:    fmt.Sprintf("The chart type %s does not exist. Please provide a valid chart type and refer to the documentation if you have any questions.", args...),
		}
	case errors.Is(err, InvalidCountryCodeBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       InvalidCountryCodeBusinessError.Error(),
			Title:      "Invalid Country Code",
			Message:    "The provided country code in the 'address.country' field does not conform to the ISO-3166 alpha-2 standard. Please provide a valid alpha-2 country code.",
		}
	case errors.Is(err, InvalidCodeFormatBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       InvalidCodeFormatBusinessError.Error(),
			Title:      "Invalid Code Format",
			Message:    "The 'code' field must be alphanumeric, in upper case, and must contain at least one letter. Please provide a valid code.",
		}
	case errors.Is(err, AssetCodeNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       AssetCodeNotFoundBusinessError.Error(),
			Title:      "Asset Code Not Found",
			Message:    "The provided asset code does not exist in our records. Please verify the asset code and try again.",
		}
	case errors.Is(err, PortfolioIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       PortfolioIDNotFoundBusinessError.Error(),
			Title:      "Portfolio ID Not Found",
			Message:    "The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again.",
		}
	case errors.Is(err, ProductIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       ProductIDNotFoundBusinessError.Error(),
			Title:      "Product ID Not Found",
			Message:    "The provided product ID does not exist in our records. Please verify the product ID and try again.",
		}
	case errors.Is(err, LedgerIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       LedgerIDNotFoundBusinessError.Error(),
			Title:      "Ledger ID Not Found",
			Message:    "The provided ledger ID does not exist in our records. Please verify the ledger ID and try again.",
		}
	case errors.Is(err, OrganizationIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       OrganizationIDNotFoundBusinessError.Error(),
			Title:      "Organization ID Not Found",
			Message:    "The provided organization ID does not exist in our records. Please verify the organization ID and try again.",
		}
	case errors.Is(err, ParentOrganizationIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       ParentOrganizationIDNotFoundBusinessError.Error(),
			Title:      "Parent Organization ID Not Found",
			Message:    "The provided parent organization ID does not exist in our records. Please verify the parent organization ID and try again.",
		}
	case errors.Is(err, InvalidTypeBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       InvalidTypeBusinessError.Error(),
			Title:      "Invalid Type",
			Message:    "The provided 'type' is not valid. Accepted types are currency, crypto, commodities, or others. Please provide a valid type.",
		}
	case errors.Is(err, TokenMissingBusinessError):
		return common.UnauthorizedError{
			EntityType: entityType,
			Code:       TokenMissingBusinessError.Error(),
			Title:      "Token Missing",
			Message:    "A valid token must be provided in the request header. Please include a token and try again.",
		}
	case errors.Is(err, InvalidTokenBusinessError):
		return common.UnauthorizedError{
			EntityType: entityType,
			Code:       InvalidTokenBusinessError.Error(),
			Title:      "Invalid Token",
			Message:    "The provided token is expired, invalid or malformed. Please provide a valid token and try again.",
		}
	case errors.Is(err, InsufficientPrivilegesBusinessError):
		return common.ForbiddenError{
			EntityType: entityType,
			Code:       InsufficientPrivilegesBusinessError.Error(),
			Title:      "Insufficient Privileges",
			Message:    "You do not have the necessary permissions to perform this action. Please contact your administrator if you believe this is an error.",
		}
	case errors.Is(err, PermissionEnforcementBusinessError):
		return common.FailedPreconditionError{
			EntityType: entityType,
			Code:       PermissionEnforcementBusinessError.Error(),
			Title:      "Permission Enforcement Error",
			Message:    "The enforcer is not configured properly. Please contact your administrator if you believe this is an error.",
		}
	case errors.Is(err, JWKFetchBusinessError):
		return common.FailedPreconditionError{
			EntityType: entityType,
			Code:       JWKFetchBusinessError.Error(),
			Title:      "JWK Fetch Error",
			Message:    "The JWK keys could not be fetched from the source. Please verify the source environment variable configuration and try again.",
		}
	case errors.Is(err, InvalidDSLFileFormatBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       InvalidDSLFileFormatBusinessError.Error(),
			Title:      "Invalid DSL File Format",
			Message:    fmt.Sprintf("The submitted DSL file %s is in an incorrect format. Please ensure that the file follows the expected structure and syntax.", args...),
		}
	case errors.Is(err, EmptyDSLFileBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       EmptyDSLFileBusinessError.Error(),
			Title:      "Empty DSL File",
			Message:    fmt.Sprintf("The submitted DSL file %s is empty. Please provide a valid file with content.", args...),
		}
	case errors.Is(err, MetadataKeyLengthExceededBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       MetadataKeyLengthExceededBusinessError.Error(),
			Title:      "Metadata Key Length Exceeded",
			Message:    fmt.Sprintf("The metadata key %s exceeds the maximum allowed length of 100 characters. Please use a shorter key.", args...),
		}
	case errors.Is(err, MetadataValueLengthExceededBusinessError):
		return common.ValidationError{
			EntityType: entityType,
			Code:       MetadataValueLengthExceededBusinessError.Error(),
			Title:      "Metadata Value Length Exceeded",
			Message:    fmt.Sprintf("The metadata value %s exceeds the maximum allowed length of 100 characters. Please use a shorter value.", args...),
		}
	case errors.Is(err, AccountIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       AccountIDNotFoundBusinessError.Error(),
			Title:      "Account ID Not Found",
			Message:    "The provided account ID does not exist in our records. Please verify the account ID and try again.",
		}
	case errors.Is(err, IDsNotFoundForAccountsBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       IDsNotFoundForAccountsBusinessError.Error(),
			Title:      "IDs Not Found for Accounts",
			Message:    "No accounts were found for the provided IDs. Please verify the IDs and try again.",
		}
	case errors.Is(err, AssetIDNotFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       AssetIDNotFoundBusinessError.Error(),
			Title:      "Asset ID Not Found",
			Message:    "The provided asset ID does not exist in our records. Please verify the asset ID and try again.",
		}
	case errors.Is(err, NoAssetsFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       NoAssetsFoundBusinessError.Error(),
			Title:      "No Assets Found",
			Message:    "No assets were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, NoProductsFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       NoProductsFoundBusinessError.Error(),
			Title:      "No Products Found",
			Message:    "No products were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, NoPortfoliosFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       NoPortfoliosFoundBusinessError.Error(),
			Title:      "No Portfolios Found",
			Message:    "No portfolios were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, NoOrganizationsFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       NoOrganizationsFoundBusinessError.Error(),
			Title:      "No Organizations Found",
			Message:    "No organizations were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, NoLedgersFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       NoLedgersFoundBusinessError.Error(),
			Title:      "No Ledgers Found",
			Message:    "No ledgers were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, BalanceUpdateFailedBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       BalanceUpdateFailedBusinessError.Error(),
			Title:      "Balance Update Failed",
			Message:    "The balance could not be updated for the specified account ID. Please verify the account ID and try again.",
		}
	case errors.Is(err, NoAccountIDsProvidedBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       NoAccountIDsProvidedBusinessError.Error(),
			Title:      "No Account IDs Provided",
			Message:    "No account IDs were provided for the balance update. Please provide valid account IDs and try again.",
		}
	case errors.Is(err, FailedToRetrieveAccountsByAliasesBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       FailedToRetrieveAccountsByAliasesBusinessError.Error(),
			Title:      "Failed To Retrieve Accounts By Aliases",
			Message:    "The accounts could not be retrieved using the specified aliases. Please verify the aliases for accuracy and try again.",
		}
	case errors.Is(err, NoAccountsFoundBusinessError):
		return common.EntityNotFoundError{
			EntityType: entityType,
			Code:       NoAccountsFoundBusinessError.Error(),
			Title:      "No Accounts Found",
			Message:    "No accounts were found in the search. Please review the search criteria and try again.",
		}
	default:
		return err
	}
}
