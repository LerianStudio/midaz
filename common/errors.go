package common

import (
	"errors"
	"fmt"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/net/http"
	"strings"
)

// EntityNotFoundError records an error indicating an entity was not found in any case that caused it.
// You can use it to representing a Database not found, cache not found or any other repository.
type EntityNotFoundError struct {
	EntityType string
	Title      string
	Message    string
	Code       string
	Err        error
}

// NewEntityNotFoundError creates an instance of EntityNotFoundError.
func NewEntityNotFoundError(entityType string) EntityNotFoundError {
	return EntityNotFoundError{
		EntityType: entityType,
		Code:       "",
		Title:      "",
		Message:    "",
		Err:        nil,
	}
}

// WrapEntityNotFoundError creates an instance of EntityNotFoundError.
func WrapEntityNotFoundError(entityType string, err error) EntityNotFoundError {
	return EntityNotFoundError{
		EntityType: entityType,
		Code:       "",
		Title:      "",
		Message:    "",
		Err:        err,
	}
}

// Error implements the error interface.
func (e EntityNotFoundError) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		if strings.TrimSpace(e.EntityType) != "" {
			return fmt.Sprintf("Entity %s not found", e.EntityType)
		}

		if e.Err != nil && strings.TrimSpace(e.Message) == "" {
			return e.Err.Error()
		}

		return "entity not found"
	}

	return e.Message
}

// Unwrap implements the error interface introduced in Go 1.13 to unwrap the internal error.
func (e EntityNotFoundError) Unwrap() error {
	return e.Err
}

// ValidationError records an error indicating an entity was not found in any case that caused it.
// You can use it to representing a Database not found, cache not found or any other repository.
type ValidationError struct {
	EntityType string
	Title      string
	Message    string
	Code       string
	Err        error
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	if strings.TrimSpace(e.Code) != "" {
		return fmt.Sprintf("%s - %s", e.Code, e.Message)
	}

	return e.Message
}

// Unwrap implements the error interface introduced in Go 1.13 to unwrap the internal error.
func (e ValidationError) Unwrap() error {
	return e.Err
}

// EntityConflictError records an error indicating an entity already exists in some repository
// You can use it to representing a Database conflict, cache or any other repository.
type EntityConflictError struct {
	EntityType string
	Title      string
	Message    string
	Code       string
	Err        error
}

// Error implements the error interface.
func (e EntityConflictError) Error() string {
	if e.Err != nil && strings.TrimSpace(e.Message) == "" {
		return e.Err.Error()
	}

	return e.Message
}

// Unwrap implements the error interface introduced in Go 1.13 to unwrap the internal error.
func (e EntityConflictError) Unwrap() error {
	return e.Err
}

// UnauthorizedError indicates an operation that couldn't be performant because there's no user authenticated.
type UnauthorizedError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
}

func (e UnauthorizedError) Error() string {
	return e.Message
}

// ForbiddenError indicates an operation that couldn't be performant because the authenticated user has no sufficient privileges.
type ForbiddenError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
}

func (e ForbiddenError) Error() string {
	return e.Message
}

// UnprocessableOperationError indicates an operation that couldn't be performant because it's invalid.
type UnprocessableOperationError struct {
	EntityType string
	Title      string
	Message    string
	Code       string
	Err        error
}

func (e UnprocessableOperationError) Error() string {
	return e.Message
}

// HTTPError indicates a http error raised in a http client.
type HTTPError struct {
	EntityType string
	Title      string
	Message    string
	Code       string
	Err        error
}

func (e HTTPError) Error() string {
	return e.Message
}

// FailedPreconditionError indicates a precondition failed during an operation.
type FailedPreconditionError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
}

func (e FailedPreconditionError) Error() string {
	return e.Message
}

// InternalServerError indicates a precondition failed during an operation.
type InternalServerError struct {
	EntityType string `json:"entityType,omitempty"`
	Title      string `json:"title,omitempty"`
	Message    string `json:"message,omitempty"`
	Code       string `json:"code,omitempty"`
	Err        error  `json:"err,omitempty"`
}

func (e InternalServerError) Error() string {
	return e.Message
}

// ValidateInternalError validate the error and return the appropriate internal error code, title and message
func ValidateInternalError(err error, entityType string) error {
	return InternalServerError{
		EntityType: entityType,
		Code:       cn.InternalServerBusinessError.Error(),
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
			Code:       cn.UnexpectedFieldsInTheRequestBusinessError.Error(),
			Title:      "Unexpected Fields in the Request",
			Message:    "The request body contains more fields than expected. Please send only the allowed fields as per the documentation. The unexpected fields are listed in the fields object.",
			Fields:     unknownFields,
		}
	}

	return http.ValidationKnownFieldsError{
		EntityType: entityType,
		Code:       cn.BadRequestBusinessError.Error(),
		Title:      "Bad Request",
		Message:    "The server could not understand the request due to malformed syntax. Please check the listed fields and try again.",
		Fields:     knownInvalidFields,
	}
}

// ValidateBusinessError validate the error and return the appropriate business error code, title and message
func ValidateBusinessError(err error, entityType string, args ...interface{}) error {
	switch {
	case errors.Is(err, cn.DuplicateLedgerBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.DuplicateLedgerBusinessError.Error(),
			Title:      "Duplicate Ledger Error",
			Message:    fmt.Sprintf("A ledger with the name %s already exists in the division %s. Please rename the ledger or choose a different division to attach it to.", args...),
		}
	case errors.Is(err, cn.LedgerNameConflictBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.LedgerNameConflictBusinessError.Error(),
			Title:      "Ledger Name Conflict",
			Message:    fmt.Sprintf("A ledger named %s already exists in your organization. Please rename the ledger, or if you want to use the same name, consider creating a new ledger for a different division.", args...),
		}
	case errors.Is(err, cn.AssetNameOrCodeDuplicateBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.AssetNameOrCodeDuplicateBusinessError.Error(),
			Title:      "Asset Name or Code Duplicate",
			Message:    "An asset with the same name or code already exists in your ledger. Please modify the name or code of your new asset.",
		}
	case errors.Is(err, cn.CodeUppercaseRequirementBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.CodeUppercaseRequirementBusinessError.Error(),
			Title:      "Code Uppercase Requirement",
			Message:    "The code must be in uppercase. Please ensure that the code is in uppercase format and try again.",
		}
	case errors.Is(err, cn.CurrencyCodeStandardComplianceBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.CurrencyCodeStandardComplianceBusinessError.Error(),
			Title:      "Currency Code Standard Compliance",
			Message:    "Currency-type assets must comply with the ISO-4217 standard. Please use a currency code that conforms to ISO-4217 guidelines.",
		}
	case errors.Is(err, cn.UnmodifiableFieldBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.UnmodifiableFieldBusinessError.Error(),
			Title:      "Unmodifiable Field Error",
			Message:    "Your request includes a field that cannot be modified. Please review your request and try again, removing any uneditable fields. Please refer to the documentation for guidance.",
		}
	case errors.Is(err, cn.EntityNotFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.EntityNotFoundBusinessError.Error(),
			Title:      "Entity Not Found",
			Message:    "No entity was found for the given ID. Please make sure to use the correct ID for the entity you are trying to manage.",
		}
	case errors.Is(err, cn.ActionNotPermittedBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ActionNotPermittedBusinessError.Error(),
			Title:      "Action Not Permitted",
			Message:    "The action you are attempting is not allowed in the current environment. Please refer to the documentation for guidance.",
		}
	case errors.Is(err, cn.MissingFieldsInRequestBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.MissingFieldsInRequestBusinessError.Error(),
			Title:      "Missing Fields in Request",
			Message:    "Your request is missing one or more required fields. Please refer to the documentation to ensure all necessary fields are included in your request.",
		}
	case errors.Is(err, cn.AccountTypeImmutableBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.AccountTypeImmutableBusinessError.Error(),
			Title:      "Account Type Immutable",
			Message:    "The account type specified cannot be modified. Please ensure the correct account type is being used and try again.",
		}
	case errors.Is(err, cn.InactiveAccountTypeBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.InactiveAccountTypeBusinessError.Error(),
			Title:      "Inactive Account Type Error",
			Message:    "The account type specified cannot be set to INACTIVE. Please ensure the correct account type is being used and try again.",
		}
	case errors.Is(err, cn.AccountBalanceDeletionBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.AccountBalanceDeletionBusinessError.Error(),
			Title:      "Account Balance Deletion Error",
			Message:    "An account or sub-account cannot be deleted if it has a remaining balance. Please ensure all remaining balances are transferred to another account before attempting to delete.",
		}
	case errors.Is(err, cn.ResourceAlreadyDeletedBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ResourceAlreadyDeletedBusinessError.Error(),
			Title:      "Resource Already Deleted",
			Message:    "The resource you are trying to delete has already been deleted. Ensure you are using the correct ID and try again.",
		}
	case errors.Is(err, cn.ProductIDInactiveBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ProductIDInactiveBusinessError.Error(),
			Title:      "Product ID Inactive",
			Message:    "The Product ID you are attempting to use is inactive. Please use another Product ID and try again.",
		}
	case errors.Is(err, cn.DuplicateProductNameBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.DuplicateProductNameBusinessError.Error(),
			Title:      "Duplicate Product Name Error",
			Message:    fmt.Sprintf("A product with the name %s already exists for this ledger ID %s. Please try again with a different ledger or name.", args...),
		}
	case errors.Is(err, cn.BalanceRemainingDeletionBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.BalanceRemainingDeletionBusinessError.Error(),
			Title:      "Balance Remaining Deletion Error",
			Message:    "The asset cannot be deleted because there is a remaining balance. Please ensure all balances are cleared before attempting to delete again.",
		}
	case errors.Is(err, cn.InvalidScriptFormatBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.InvalidScriptFormatBusinessError.Error(),
			Title:      "Invalid Script Format Error",
			Message:    "The script provided in your request is invalid or in an unsupported format. Please verify the script format and try again.",
		}
	case errors.Is(err, cn.InsufficientFundsBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.InsufficientFundsBusinessError.Error(),
			Title:      "Insufficient Funds Error",
			Message:    "The transaction could not be completed due to insufficient funds in the account. Please add sufficient funds to your account and try again.",
		}
	case errors.Is(err, cn.AccountIneligibilityBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.AccountIneligibilityBusinessError.Error(),
			Title:      "Account Ineligibility Error",
			Message:    "One or more accounts listed in the transaction are not eligible to participate. Please review the account statuses and try again.",
		}
	case errors.Is(err, cn.AliasUnavailabilityBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.AliasUnavailabilityBusinessError.Error(),
			Title:      "Alias Unavailability Error",
			Message:    fmt.Sprintf("The alias %s is already in use. Please choose a different alias and try again.", args...),
		}
	case errors.Is(err, cn.ParentTransactionIDNotFoundBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ParentTransactionIDNotFoundBusinessError.Error(),
			Title:      "Parent Transaction ID Not Found",
			Message:    fmt.Sprintf("The parentTransactionId %s does not correspond to any existing transaction. Please review the ID and try again.", args...),
		}
	case errors.Is(err, cn.ImmutableFieldBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.ImmutableFieldBusinessError.Error(),
			Title:      "Immutable Field Error",
			Message:    fmt.Sprintf("The %s field cannot be modified. Please remove this field from your request and try again.", args...),
		}
	case errors.Is(err, cn.TransactionTimingRestrictionBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.TransactionTimingRestrictionBusinessError.Error(),
			Title:      "Transaction Timing Restriction",
			Message:    fmt.Sprintf("You can only perform another transaction using %s of %f from %s to %s after %s. Please wait until the specified time to try again.", args...),
		}
	case errors.Is(err, cn.AccountStatusTransactionRestrictionBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.AccountStatusTransactionRestrictionBusinessError.Error(),
			Title:      "Account Status Transaction Restriction",
			Message:    "The current statuses of the source and/or destination accounts do not permit transactions. Change the account status(es) and try again.",
		}
	case errors.Is(err, cn.InsufficientAccountBalanceBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.InsufficientAccountBalanceBusinessError.Error(),
			Title:      "Insufficient Account Balance Error",
			Message:    fmt.Sprintf("The account %s does not have sufficient balance. Please try again with an amount that is less than or equal to the available balance.", args...),
		}
	case errors.Is(err, cn.TransactionMethodRestrictionBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.TransactionMethodRestrictionBusinessError.Error(),
			Title:      "Transaction Method Restriction",
			Message:    fmt.Sprintf("Transactions involving %s are not permitted for the specified source and/or destination. Please try again using accounts that allow transactions with %s.", args...),
		}
	case errors.Is(err, cn.DuplicateTransactionTemplateCodeBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.DuplicateTransactionTemplateCodeBusinessError.Error(),
			Title:      "Duplicate Transaction Template Code Error",
			Message:    fmt.Sprintf("A transaction template with the code %s already exists for your ledger. Please use a different code and try again.", args...),
		}
	case errors.Is(err, cn.DuplicateAssetPairBusinessError):
		return EntityConflictError{
			EntityType: entityType,
			Code:       cn.DuplicateAssetPairBusinessError.Error(),
			Title:      "Duplicate Asset Pair Error",
			Message:    fmt.Sprintf("A pair for the assets %s%s already exists with the ID %s. Please update the existing entry instead of creating a new one.", args...),
		}
	case errors.Is(err, cn.InvalidParentAccountIDBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.InvalidParentAccountIDBusinessError.Error(),
			Title:      "Invalid Parent Account ID",
			Message:    "The specified parent account ID does not exist. Please verify the ID is correct and attempt your request again.",
		}
	case errors.Is(err, cn.MismatchedAssetCodeBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.MismatchedAssetCodeBusinessError.Error(),
			Title:      "Mismatched Asset Code",
			Message:    "The parent account ID you provided is associated with a different asset code than the one specified in your request. Please make sure the asset code matches that of the parent account, or use a different parent account ID and try again.",
		}
	case errors.Is(err, cn.ChartTypeNotFoundBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.ChartTypeNotFoundBusinessError.Error(),
			Title:      "Chart Type Not Found",
			Message:    fmt.Sprintf("The chart type %s does not exist. Please provide a valid chart type and refer to the documentation if you have any questions.", args...),
		}
	case errors.Is(err, cn.InvalidCountryCodeBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.InvalidCountryCodeBusinessError.Error(),
			Title:      "Invalid Country Code",
			Message:    "The provided country code in the 'address.country' field does not conform to the ISO-3166 alpha-2 standard. Please provide a valid alpha-2 country code.",
		}
	case errors.Is(err, cn.InvalidCodeFormatBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.InvalidCodeFormatBusinessError.Error(),
			Title:      "Invalid Code Format",
			Message:    "The 'code' field must be alphanumeric, in upper case, and must contain at least one letter. Please provide a valid code.",
		}
	case errors.Is(err, cn.AssetCodeNotFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.AssetCodeNotFoundBusinessError.Error(),
			Title:      "Asset Code Not Found",
			Message:    "The provided asset code does not exist in our records. Please verify the asset code and try again.",
		}
	case errors.Is(err, cn.PortfolioIDNotFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.PortfolioIDNotFoundBusinessError.Error(),
			Title:      "Portfolio ID Not Found",
			Message:    "The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again.",
		}
	case errors.Is(err, cn.ProductIDNotFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ProductIDNotFoundBusinessError.Error(),
			Title:      "Product ID Not Found",
			Message:    "The provided product ID does not exist in our records. Please verify the product ID and try again.",
		}
	case errors.Is(err, cn.LedgerIDNotFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.LedgerIDNotFoundBusinessError.Error(),
			Title:      "Ledger ID Not Found",
			Message:    "The provided ledger ID does not exist in our records. Please verify the ledger ID and try again.",
		}
	case errors.Is(err, cn.OrganizationIDNotFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.OrganizationIDNotFoundBusinessError.Error(),
			Title:      "Organization ID Not Found",
			Message:    "The provided organization ID does not exist in our records. Please verify the organization ID and try again.",
		}
	case errors.Is(err, cn.ParentOrganizationIDNotFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.ParentOrganizationIDNotFoundBusinessError.Error(),
			Title:      "Parent Organization ID Not Found",
			Message:    "The provided parent organization ID does not exist in our records. Please verify the parent organization ID and try again.",
		}
	case errors.Is(err, cn.InvalidTypeBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.InvalidTypeBusinessError.Error(),
			Title:      "Invalid Type",
			Message:    "The provided 'type' is not valid. Accepted types are currency, crypto, commodities, or others. Please provide a valid type.",
		}
	case errors.Is(err, cn.TokenMissingBusinessError):
		return UnauthorizedError{
			EntityType: entityType,
			Code:       cn.TokenMissingBusinessError.Error(),
			Title:      "Token Missing",
			Message:    "A valid token must be provided in the request header. Please include a token and try again.",
		}
	case errors.Is(err, cn.InvalidTokenBusinessError):
		return UnauthorizedError{
			EntityType: entityType,
			Code:       cn.InvalidTokenBusinessError.Error(),
			Title:      "Invalid Token",
			Message:    "The provided token is expired, invalid or malformed. Please provide a valid token and try again.",
		}
	case errors.Is(err, cn.InsufficientPrivilegesBusinessError):
		return ForbiddenError{
			EntityType: entityType,
			Code:       cn.InsufficientPrivilegesBusinessError.Error(),
			Title:      "Insufficient Privileges",
			Message:    "You do not have the necessary permissions to perform this action. Please contact your administrator if you believe this is an error.",
		}
	case errors.Is(err, cn.PermissionEnforcementBusinessError):
		return FailedPreconditionError{
			EntityType: entityType,
			Code:       cn.PermissionEnforcementBusinessError.Error(),
			Title:      "Permission Enforcement Error",
			Message:    "The enforcer is not configured properly. Please contact your administrator if you believe this is an error.",
		}
	case errors.Is(err, cn.JWKFetchBusinessError):
		return FailedPreconditionError{
			EntityType: entityType,
			Code:       cn.JWKFetchBusinessError.Error(),
			Title:      "JWK Fetch Error",
			Message:    "The JWK keys could not be fetched from the source. Please verify the source environment variable configuration and try again.",
		}
	case errors.Is(err, cn.InvalidDSLFileFormatBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.InvalidDSLFileFormatBusinessError.Error(),
			Title:      "Invalid DSL File Format",
			Message:    fmt.Sprintf("The submitted DSL file %s is in an incorrect format. Please ensure that the file follows the expected structure and syntax.", args...),
		}
	case errors.Is(err, cn.EmptyDSLFileBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.EmptyDSLFileBusinessError.Error(),
			Title:      "Empty DSL File",
			Message:    fmt.Sprintf("The submitted DSL file %s is empty. Please provide a valid file with content.", args...),
		}
	case errors.Is(err, cn.MetadataKeyLengthExceededBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.MetadataKeyLengthExceededBusinessError.Error(),
			Title:      "Metadata Key Length Exceeded",
			Message:    fmt.Sprintf("The metadata key %s exceeds the maximum allowed length of 100 characters. Please use a shorter key.", args...),
		}
	case errors.Is(err, cn.MetadataValueLengthExceededBusinessError):
		return ValidationError{
			EntityType: entityType,
			Code:       cn.MetadataValueLengthExceededBusinessError.Error(),
			Title:      "Metadata Value Length Exceeded",
			Message:    fmt.Sprintf("The metadata value %s exceeds the maximum allowed length of 100 characters. Please use a shorter value.", args...),
		}
	case errors.Is(err, cn.AccountIDNotFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.AccountIDNotFoundBusinessError.Error(),
			Title:      "Account ID Not Found",
			Message:    "The provided account ID does not exist in our records. Please verify the account ID and try again.",
		}
	case errors.Is(err, cn.IDsNotFoundForAccountsBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.IDsNotFoundForAccountsBusinessError.Error(),
			Title:      "IDs Not Found for Accounts",
			Message:    "No accounts were found for the provided IDs. Please verify the IDs and try again.",
		}
	case errors.Is(err, cn.AssetIDNotFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.AssetIDNotFoundBusinessError.Error(),
			Title:      "Asset ID Not Found",
			Message:    "The provided asset ID does not exist in our records. Please verify the asset ID and try again.",
		}
	case errors.Is(err, cn.NoAssetsFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.NoAssetsFoundBusinessError.Error(),
			Title:      "No Assets Found",
			Message:    "No assets were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, cn.NoProductsFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.NoProductsFoundBusinessError.Error(),
			Title:      "No Products Found",
			Message:    "No products were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, cn.NoPortfoliosFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.NoPortfoliosFoundBusinessError.Error(),
			Title:      "No Portfolios Found",
			Message:    "No portfolios were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, cn.NoOrganizationsFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.NoOrganizationsFoundBusinessError.Error(),
			Title:      "No Organizations Found",
			Message:    "No organizations were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, cn.NoLedgersFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.NoLedgersFoundBusinessError.Error(),
			Title:      "No Ledgers Found",
			Message:    "No ledgers were found in the search. Please review the search criteria and try again.",
		}
	case errors.Is(err, cn.BalanceUpdateFailedBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.BalanceUpdateFailedBusinessError.Error(),
			Title:      "Balance Update Failed",
			Message:    "The balance could not be updated for the specified account ID. Please verify the account ID and try again.",
		}
	case errors.Is(err, cn.NoAccountIDsProvidedBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.NoAccountIDsProvidedBusinessError.Error(),
			Title:      "No Account IDs Provided",
			Message:    "No account IDs were provided for the balance update. Please provide valid account IDs and try again.",
		}
	case errors.Is(err, cn.FailedToRetrieveAccountsByAliasesBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.FailedToRetrieveAccountsByAliasesBusinessError.Error(),
			Title:      "Failed To Retrieve Accounts By Aliases",
			Message:    "The accounts could not be retrieved using the specified aliases. Please verify the aliases for accuracy and try again.",
		}
	case errors.Is(err, cn.NoAccountsFoundBusinessError):
		return EntityNotFoundError{
			EntityType: entityType,
			Code:       cn.NoAccountsFoundBusinessError.Error(),
			Title:      "No Accounts Found",
			Message:    "No accounts were found in the search. Please review the search criteria and try again.",
		}
	default:
		return err
	}
}
