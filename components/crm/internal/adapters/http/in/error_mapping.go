package in

import "github.com/LerianStudio/midaz/v3/pkg/constant"

// CRMErrorMapping maps generic Midaz error codes to CRM-specific error codes.
// This ensures backward compatibility for CRM API clients after the migration
// from a standalone repository to the Midaz monorepo.
//
// The mapping only covers errors that originate from shared packages (e.g., validation).
var CRMErrorMapping = map[string]string{
	// Validation errors from pkg/net/http/withBody.go
	constant.ErrInvalidMetadataNesting.Error():       constant.ErrInvalidMetadataNestingCRM.Error(),       // 0067 → CRM-0001
	constant.ErrMetadataKeyLengthExceeded.Error():    constant.ErrMetadataKeyLengthExceededCRM.Error(),    // 0050 → CRM-0002
	constant.ErrMissingFieldsInRequest.Error():       constant.ErrMissingFieldsInRequestCRM.Error(),       // 0009 → CRM-0003
	constant.ErrInvalidPathParameter.Error():         constant.ErrInvalidPathParameterCRM.Error(),         // 0065 → CRM-0005
	constant.ErrUnexpectedFieldsInTheRequest.Error(): constant.ErrUnexpectedFieldsInTheRequestCRM.Error(), // 0053 → CRM-0007
	constant.ErrPaginationLimitExceeded.Error():      constant.ErrPaginationLimitExceededCRM.Error(),      // 0080 → CRM-0009
	constant.ErrInvalidSortOrder.Error():             constant.ErrInvalidSortOrderCRM.Error(),             // 0081 → CRM-0011
	constant.ErrMetadataValueLengthExceeded.Error():  constant.ErrMetadataValueLengthExceededCRM.Error(),  // 0051 → CRM-0012
	constant.ErrInternalServer.Error():               constant.ErrInternalServerCRM.Error(),               // 0046 → CRM-0014
	constant.ErrBadRequest.Error():                   constant.ErrBadRequestCRM.Error(),                   // 0047 → CRM-0015
	constant.ErrInvalidQueryParameter.Error():        constant.ErrInvalidQueryParameterCRM.Error(),        // 0082 → CRM-0016
	constant.ErrInvalidRequestBody.Error():           constant.ErrInvalidFieldTypeInRequest.Error(),       // 0094 → CRM-0004
}

// TransformErrorCode transforms a generic error code to a CRM-specific error code
// if a mapping exists. Returns the original code if no mapping is found.
func TransformErrorCode(code string) string {
	if crmCode, exists := CRMErrorMapping[code]; exists {
		return crmCode
	}

	return code
}
