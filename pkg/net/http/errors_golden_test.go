// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// This file is the MONEY-PATH golden net for the (code, HTTP status) table.
//
// It drives every error through the REAL dispatcher (WithError via fiber, or
// CanonicalFiberErrorHandler for the two explicit-status arms) and asserts ONLY
// resp.StatusCode and body["code"] — the two values a client's error handling keys
// off. The rest of the envelope is deliberately left unasserted: title and detail
// are allowed to move (the >=500 scrub rewrites both), so pinning them here would
// turn a body change that leaves the money-path tuple intact into a RED test.
//
// The expected (status, code) for each case is derived by classifyStatusOf: the
// SAME errors.As cascade over the SAME typed structs, in the SAME declaration
// order, that the dispatcher walks. Status is a pure function of the Go error TYPE
// that ValidateBusinessError picked for the sentinel, never of the numeric code, so
// a cascade over the types IS the whole table. Because the sweep re-derives the
// expected value from that classifier and compares it to what the live dispatcher
// emits, the test is self-generating and drift-proof: add a code, it is swept;
// change a code's type/status, this test goes RED.
//
// ponytail: one classifier, swept over every sentinel — the smallest thing that
// fails if a code or status drifts.

// classifyStatusOf is an INDEPENDENT restatement of the dispatcher's type->status
// table: the same errors.As cascade over the same typed structs, in the same
// declaration order, first match wins, returning the (HTTP status, code) that arm
// emits. It is restated rather than called so a change to the production cascade
// shows up here as a disagreement instead of being mirrored into the expectation.
// On no match it returns the fallback the dispatcher applies to an error it cannot
// classify: 500 with the internal-server code "0046".
//
// ResponseError is the status-in-Code quirk: its status is strconv.Atoi(Code)
// (response.go:124), so it is derived here, not from a fixed HTTP status.
func classifyStatusOf(t *testing.T, err error) (status int, code string) {
	t.Helper()

	if e := (pkg.EntityNotFoundError{}); errors.As(err, &e) {
		return fiber.StatusNotFound, e.Code
	}
	if e := (pkg.EntityConflictError{}); errors.As(err, &e) {
		return fiber.StatusConflict, e.Code
	}
	if e := (pkg.ValidationError{}); errors.As(err, &e) {
		return fiber.StatusBadRequest, e.Code
	}
	if e := (pkg.UnprocessableOperationError{}); errors.As(err, &e) {
		return fiber.StatusUnprocessableEntity, e.Code
	}
	if e := (pkg.UnauthorizedError{}); errors.As(err, &e) {
		return fiber.StatusUnauthorized, e.Code
	}
	if e := (pkg.ForbiddenError{}); errors.As(err, &e) {
		return fiber.StatusForbidden, e.Code
	}
	if e := (pkg.ValidationKnownFieldsError{}); errors.As(err, &e) {
		return fiber.StatusBadRequest, e.Code
	}
	if e := (pkg.ValidationUnknownFieldsError{}); errors.As(err, &e) {
		return fiber.StatusBadRequest, e.Code
	}
	if e := (pkg.ResponseError{}); errors.As(err, &e) {
		n, convErr := strconv.Atoi(e.Code)
		require.NoError(t, convErr, "ResponseError.Code must parse as the HTTP status integer")

		return n, e.Code
	}
	if e := (pkg.InternalServerError{}); errors.As(err, &e) {
		return fiber.StatusInternalServerError, e.Code
	}
	if e := (pkg.FailedPreconditionError{}); errors.As(err, &e) {
		// 500, NOT the 412 the name suggests: the arm renders through the same
		// internal-server helper as InternalServerError.
		return fiber.StatusInternalServerError, e.Code
	}
	if e := (pkg.ServiceUnavailableError{}); errors.As(err, &e) {
		return fiber.StatusServiceUnavailable, e.Code
	}
	if e := (pkg.GatewayTimeoutError{}); errors.As(err, &e) {
		return fiber.StatusGatewayTimeout, e.Code
	}
	if e := (pkg.PayloadTooLargeError{}); errors.As(err, &e) {
		return fiber.StatusRequestEntityTooLarge, e.Code
	}

	// Fallthrough: WithError:110 -> ValidateInternalError -> 500 / code 0046.
	return fiber.StatusInternalServerError, constant.ErrInternalServer.Error()
}

// driveWithError sends err through the live WithError dispatcher and returns the
// emitted (status, body["code"]).
//
// CanonicalFiberErrorHandler is installed as the app ErrorHandler because that is
// how every Midaz fiber app is wired (fiber_error_handler.go:20-27): WithError's
// fallthrough arm (errors.go:110) RETURNS an unwritten *InternalServerError, and
// only the ErrorHandler renders it as the canonical JSON envelope. Without it,
// fiber's default handler stringifies the error to plain text and body["code"]
// cannot be read — so this harness must carry the real render chain, not half of
// it.
func driveWithError(t *testing.T, err error) (status int, code string) {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: CanonicalFiberErrorHandler,
	})
	app.Get("/probe", func(c fiber.Ctx) error { return WithError(c, err) })

	resp, testErr := app.Test(httptest.NewRequest(fiber.MethodGet, "/probe", nil))
	require.NoError(t, testErr)

	defer func() { _ = resp.Body.Close() }()

	b, readErr := io.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var body map[string]any
	require.NoError(t, json.Unmarshal(b, &body), "body must be JSON, got: %s", string(b))

	codeVal, _ := body["code"].(string)

	return resp.StatusCode, codeVal
}

// allSentinels is every constant.Err* sentinel declared in pkg/constant, keyed by its Go
// identifier. Kept as a literal because Go cannot reflect over package-level vars; keying by
// NAME is what lets TestGolden_SentinelInventoryComplete diff this inventory against the
// registry parsed out of that package, so a sentinel added there without being added here fails
// CI rather than silently escaping the sweep.
func allSentinels() map[string]error {
	return map[string]error{
		"ErrDuplicateLedger":                          constant.ErrDuplicateLedger,
		"ErrLedgerNameConflict":                       constant.ErrLedgerNameConflict,
		"ErrAssetNameOrCodeDuplicate":                 constant.ErrAssetNameOrCodeDuplicate,
		"ErrCodeUppercaseRequirement":                 constant.ErrCodeUppercaseRequirement,
		"ErrCurrencyCodeStandardCompliance":           constant.ErrCurrencyCodeStandardCompliance,
		"ErrUnmodifiableField":                        constant.ErrUnmodifiableField,
		"ErrEntityNotFound":                           constant.ErrEntityNotFound,
		"ErrActionNotPermitted":                       constant.ErrActionNotPermitted,
		"ErrMissingFieldsInRequest":                   constant.ErrMissingFieldsInRequest,
		"ErrAccountTypeImmutable":                     constant.ErrAccountTypeImmutable,
		"ErrInactiveAccountType":                      constant.ErrInactiveAccountType,
		"ErrAccountBalanceDeletion":                   constant.ErrAccountBalanceDeletion,
		"ErrResourceAlreadyDeleted":                   constant.ErrResourceAlreadyDeleted,
		"ErrSegmentIDInactive":                        constant.ErrSegmentIDInactive,
		"ErrDuplicateSegmentName":                     constant.ErrDuplicateSegmentName,
		"ErrInsufficientFunds":                        constant.ErrInsufficientFunds,
		"ErrAccountIneligibility":                     constant.ErrAccountIneligibility,
		"ErrAliasUnavailability":                      constant.ErrAliasUnavailability,
		"ErrParentTransactionIDNotFound":              constant.ErrParentTransactionIDNotFound,
		"ErrImmutableField":                           constant.ErrImmutableField,
		"ErrTransactionTimingRestriction":             constant.ErrTransactionTimingRestriction,
		"ErrAccountStatusTransactionRestriction":      constant.ErrAccountStatusTransactionRestriction,
		"ErrInsufficientAccountBalance":               constant.ErrInsufficientAccountBalance,
		"ErrTransactionMethodRestriction":             constant.ErrTransactionMethodRestriction,
		"ErrDuplicateTransactionTemplateCode":         constant.ErrDuplicateTransactionTemplateCode,
		"ErrDuplicateAssetPair":                       constant.ErrDuplicateAssetPair,
		"ErrInvalidParentAccountID":                   constant.ErrInvalidParentAccountID,
		"ErrMismatchedAssetCode":                      constant.ErrMismatchedAssetCode,
		"ErrChartTypeNotFound":                        constant.ErrChartTypeNotFound,
		"ErrInvalidCountryCode":                       constant.ErrInvalidCountryCode,
		"ErrInvalidCodeFormat":                        constant.ErrInvalidCodeFormat,
		"ErrAssetCodeNotFound":                        constant.ErrAssetCodeNotFound,
		"ErrPortfolioIDNotFound":                      constant.ErrPortfolioIDNotFound,
		"ErrSegmentIDNotFound":                        constant.ErrSegmentIDNotFound,
		"ErrLedgerIDNotFound":                         constant.ErrLedgerIDNotFound,
		"ErrOrganizationIDNotFound":                   constant.ErrOrganizationIDNotFound,
		"ErrParentOrganizationIDNotFound":             constant.ErrParentOrganizationIDNotFound,
		"ErrInvalidType":                              constant.ErrInvalidType,
		"ErrTokenMissing":                             constant.ErrTokenMissing,
		"ErrInvalidToken":                             constant.ErrInvalidToken,
		"ErrInsufficientPrivileges":                   constant.ErrInsufficientPrivileges,
		"ErrPermissionEnforcement":                    constant.ErrPermissionEnforcement,
		"ErrJWKFetch":                                 constant.ErrJWKFetch,
		"ErrInternalServer":                           constant.ErrInternalServer,
		"ErrBadRequest":                               constant.ErrBadRequest,
		"ErrMetadataKeyLengthExceeded":                constant.ErrMetadataKeyLengthExceeded,
		"ErrMetadataValueLengthExceeded":              constant.ErrMetadataValueLengthExceeded,
		"ErrAccountIDNotFound":                        constant.ErrAccountIDNotFound,
		"ErrUnexpectedFieldsInTheRequest":             constant.ErrUnexpectedFieldsInTheRequest,
		"ErrIDsNotFoundForAccounts":                   constant.ErrIDsNotFoundForAccounts,
		"ErrAssetIDNotFound":                          constant.ErrAssetIDNotFound,
		"ErrNoAssetsFound":                            constant.ErrNoAssetsFound,
		"ErrNoSegmentsFound":                          constant.ErrNoSegmentsFound,
		"ErrNoPortfoliosFound":                        constant.ErrNoPortfoliosFound,
		"ErrNoOrganizationsFound":                     constant.ErrNoOrganizationsFound,
		"ErrNoLedgersFound":                           constant.ErrNoLedgersFound,
		"ErrBalanceUpdateFailed":                      constant.ErrBalanceUpdateFailed,
		"ErrNoAccountIDsProvided":                     constant.ErrNoAccountIDsProvided,
		"ErrFailedToRetrieveAccountsByAliases":        constant.ErrFailedToRetrieveAccountsByAliases,
		"ErrNoAccountsFound":                          constant.ErrNoAccountsFound,
		"ErrInvalidPathParameter":                     constant.ErrInvalidPathParameter,
		"ErrInvalidAccountType":                       constant.ErrInvalidAccountType,
		"ErrInvalidMetadataNesting":                   constant.ErrInvalidMetadataNesting,
		"ErrOperationIDNotFound":                      constant.ErrOperationIDNotFound,
		"ErrNoOperationsFound":                        constant.ErrNoOperationsFound,
		"ErrTransactionIDNotFound":                    constant.ErrTransactionIDNotFound,
		"ErrNoTransactionsFound":                      constant.ErrNoTransactionsFound,
		"ErrInvalidTransactionType":                   constant.ErrInvalidTransactionType,
		"ErrTransactionValueMismatch":                 constant.ErrTransactionValueMismatch,
		"ErrForbiddenExternalAccountManipulation":     constant.ErrForbiddenExternalAccountManipulation,
		"ErrAuditRecordNotRetrieved":                  constant.ErrAuditRecordNotRetrieved,
		"ErrAuditTreeRecordNotFound":                  constant.ErrAuditTreeRecordNotFound,
		"ErrInvalidDateFormat":                        constant.ErrInvalidDateFormat,
		"ErrInvalidFinalDate":                         constant.ErrInvalidFinalDate,
		"ErrDateRangeExceedsLimit":                    constant.ErrDateRangeExceedsLimit,
		"ErrPaginationLimitExceeded":                  constant.ErrPaginationLimitExceeded,
		"ErrInvalidSortOrder":                         constant.ErrInvalidSortOrder,
		"ErrInvalidQueryParameter":                    constant.ErrInvalidQueryParameter,
		"ErrInvalidDateRange":                         constant.ErrInvalidDateRange,
		"ErrIdempotencyKey":                           constant.ErrIdempotencyKey,
		"ErrAccountAliasNotFound":                     constant.ErrAccountAliasNotFound,
		"ErrLockVersionAccountBalance":                constant.ErrLockVersionAccountBalance,
		"ErrTransactionIDHasAlreadyParentTransaction": constant.ErrTransactionIDHasAlreadyParentTransaction,
		"ErrTransactionIDIsAlreadyARevert":            constant.ErrTransactionIDIsAlreadyARevert,
		"ErrTransactionCantRevert":                    constant.ErrTransactionCantRevert,
		"ErrTransactionAmbiguous":                     constant.ErrTransactionAmbiguous,
		"ErrParentIDSameID":                           constant.ErrParentIDSameID,
		"ErrNoBalancesFound":                          constant.ErrNoBalancesFound,
		"ErrBalancesCantBeDeleted":                    constant.ErrBalancesCantBeDeleted,
		"ErrInvalidRequestBody":                       constant.ErrInvalidRequestBody,
		"ErrMessageBrokerUnavailable":                 constant.ErrMessageBrokerUnavailable,
		"ErrAccountAliasInvalid":                      constant.ErrAccountAliasInvalid,
		"ErrOverFlowInt64":                            constant.ErrOverFlowInt64,
		"ErrOnHoldExternalAccount":                    constant.ErrOnHoldExternalAccount,
		"ErrCommitTransactionNotPending":              constant.ErrCommitTransactionNotPending,
		"ErrOperationRouteTitleAlreadyExists":         constant.ErrOperationRouteTitleAlreadyExists,
		"ErrOperationRouteNotFound":                   constant.ErrOperationRouteNotFound,
		"ErrNoOperationRoutesFound":                   constant.ErrNoOperationRoutesFound,
		"ErrInvalidOperationRouteType":                constant.ErrInvalidOperationRouteType,
		"ErrMissingOperationRoutes":                   constant.ErrMissingOperationRoutes,
		"ErrTransactionRouteNotFound":                 constant.ErrTransactionRouteNotFound,
		"ErrNoTransactionRoutesFound":                 constant.ErrNoTransactionRoutesFound,
		"ErrOperationRouteLinkedToTransactionRoutes":  constant.ErrOperationRouteLinkedToTransactionRoutes,
		"ErrDuplicateAccountTypeKeyValue":             constant.ErrDuplicateAccountTypeKeyValue,
		"ErrAccountTypeNotFound":                      constant.ErrAccountTypeNotFound,
		"ErrNoAccountTypesFound":                      constant.ErrNoAccountTypesFound,
		"ErrInvalidAccountRuleType":                   constant.ErrInvalidAccountRuleType,
		"ErrInvalidAccountRuleValue":                  constant.ErrInvalidAccountRuleValue,
		"ErrCorruptedAccountRule":                     constant.ErrCorruptedAccountRule,
		"ErrTransactionRouteNotInformed":              constant.ErrTransactionRouteNotInformed,
		"ErrInvalidTransactionRouteID":                constant.ErrInvalidTransactionRouteID,
		"ErrAccountingRouteCountMismatch":             constant.ErrAccountingRouteCountMismatch,
		"ErrAccountingRouteNotFound":                  constant.ErrAccountingRouteNotFound,
		"ErrAccountingAliasValidationFailed":          constant.ErrAccountingAliasValidationFailed,
		"ErrAccountingAccountTypeValidationFailed":    constant.ErrAccountingAccountTypeValidationFailed,
		"ErrInvalidAccountTypeKeyValue":               constant.ErrInvalidAccountTypeKeyValue,
		"ErrInvalidAccountTypeDirection":              constant.ErrInvalidAccountTypeDirection,
		"ErrSchemaMigrationPending":                   constant.ErrSchemaMigrationPending,
		"ErrInvalidFutureTransactionDate":             constant.ErrInvalidFutureTransactionDate,
		"ErrInvalidPendingFutureTransactionDate":      constant.ErrInvalidPendingFutureTransactionDate,
		"ErrDuplicatedAliasKeyValue":                  constant.ErrDuplicatedAliasKeyValue,
		"ErrAdditionalBalanceNotAllowed":              constant.ErrAdditionalBalanceNotAllowed,
		"ErrInvalidTransactionNonPositiveValue":       constant.ErrInvalidTransactionNonPositiveValue,
		"ErrDefaultBalanceNotFound":                   constant.ErrDefaultBalanceNotFound,
		"ErrAccountCreationFailed":                    constant.ErrAccountCreationFailed,
		"ErrTransactionBackupCacheFailed":             constant.ErrTransactionBackupCacheFailed,
		"ErrTransactionBackupCacheMarshalFailed":      constant.ErrTransactionBackupCacheMarshalFailed,
		"ErrInvalidDatetimeFormat":                    constant.ErrInvalidDatetimeFormat,
		"ErrMetadataIndexAlreadyExists":               constant.ErrMetadataIndexAlreadyExists,
		"ErrMetadataIndexNotFound":                    constant.ErrMetadataIndexNotFound,
		"ErrMetadataIndexInvalidKey":                  constant.ErrMetadataIndexInvalidKey,
		"ErrMetadataIndexLimitExceeded":               constant.ErrMetadataIndexLimitExceeded,
		"ErrMetadataIndexCreationFailed":              constant.ErrMetadataIndexCreationFailed,
		"ErrMetadataIndexDeletionForbidden":           constant.ErrMetadataIndexDeletionForbidden,
		"ErrInvalidEntityName":                        constant.ErrInvalidEntityName,
		"ErrTransactionBackupCacheRetrievalFailed":    constant.ErrTransactionBackupCacheRetrievalFailed,
		"ErrInvalidTimestamp":                         constant.ErrInvalidTimestamp,
		"ErrNoBalanceDataAtTimestamp":                 constant.ErrNoBalanceDataAtTimestamp,
		"ErrMissingRequiredQueryParameter":            constant.ErrMissingRequiredQueryParameter,
		"ErrPayloadTooLarge":                          constant.ErrPayloadTooLarge,
		"ErrRequestHeaderFieldsTooLarge":              constant.ErrRequestHeaderFieldsTooLarge,
		"ErrJSONNestingDepthExceeded":                 constant.ErrJSONNestingDepthExceeded,
		"ErrJSONKeyCountExceeded":                     constant.ErrJSONKeyCountExceeded,
		"ErrTenantNotProvisioned":                     constant.ErrTenantNotProvisioned,
		"ErrUnknownSettingsField":                     constant.ErrUnknownSettingsField,
		"ErrInvalidSettingsFieldType":                 constant.ErrInvalidSettingsFieldType,
		"ErrSettingsRootLevelField":                   constant.ErrSettingsRootLevelField,
		"ErrRouteNotBidirectional":                    constant.ErrRouteNotBidirectional,
		"ErrMissingCounterpart":                       constant.ErrMissingCounterpart,
		"ErrDirectionRouteMismatch":                   constant.ErrDirectionRouteMismatch,
		"ErrNoSourceForAction":                        constant.ErrNoSourceForAction,
		"ErrNoDestinationForAction":                   constant.ErrNoDestinationForAction,
		"ErrInvalidRouteAction":                       constant.ErrInvalidRouteAction,
		"ErrNoRoutesForAction":                        constant.ErrNoRoutesForAction,
		"ErrTooManyOperationRoutes":                   constant.ErrTooManyOperationRoutes,
		"ErrTenantServiceSuspended":                   constant.ErrTenantServiceSuspended,
		"ErrTenantNotFound":                           constant.ErrTenantNotFound,
		"ErrTenantServiceUnavailable":                 constant.ErrTenantServiceUnavailable,
		"ErrScenarioNotAllowedForDirection":           constant.ErrScenarioNotAllowedForDirection,
		"ErrReserveGroupIncomplete":                   constant.ErrReserveGroupIncomplete,
		"ErrDirectScenarioRequired":                   constant.ErrDirectScenarioRequired,
		"ErrRevertOnlyBidirectional":                  constant.ErrRevertOnlyBidirectional,
		"ErrAccountingEntryFieldRequired":             constant.ErrAccountingEntryFieldRequired,
		"ErrOverdraftLimitExceeded":                   constant.ErrOverdraftLimitExceeded,
		"ErrDirectOperationOnInternalBalance":         constant.ErrDirectOperationOnInternalBalance,
		"ErrDeletionOfInternalBalance":                constant.ErrDeletionOfInternalBalance,
		"ErrReservedBalanceKey":                       constant.ErrReservedBalanceKey,
		"ErrInvalidBalanceDirection":                  constant.ErrInvalidBalanceDirection,
		"ErrInvalidBalanceSettings":                   constant.ErrInvalidBalanceSettings,
		"ErrOverdraftLimitBelowUsage":                 constant.ErrOverdraftLimitBelowUsage,
		"ErrStaleBalanceVersion":                      constant.ErrStaleBalanceVersion,
		"ErrUpdateOfInternalBalance":                  constant.ErrUpdateOfInternalBalance,
		"ErrInvalidSettingsFieldValue":                constant.ErrInvalidSettingsFieldValue,
		"ErrTransactionReservationDenied":             constant.ErrTransactionReservationDenied,
		"ErrTransactionReservationUnavailable":        constant.ErrTransactionReservationUnavailable,
		"ErrFeeCalculationFieldType":                  constant.ErrFeeCalculationFieldType,
		"ErrPriorityInvalid":                          constant.ErrPriorityInvalid,
		"ErrFindAccountOnMidaz":                       constant.ErrFindAccountOnMidaz,
		"ErrMinAmountGreaterThanMaxAmount":            constant.ErrMinAmountGreaterThanMaxAmount,
		"ErrNothingToUpdate":                          constant.ErrNothingToUpdate,
		"ErrDuplicatePackage":                         constant.ErrDuplicatePackage,
		"ErrFeeInvalidHeaderParameter":                constant.ErrFeeInvalidHeaderParameter,
		"ErrCalculateFee":                             constant.ErrCalculateFee,
		"ErrCalculationRequired":                      constant.ErrCalculationRequired,
		"ErrPriorityOne":                              constant.ErrPriorityOne,
		"ErrAppRuleFlatFeeAndPercentual":              constant.ErrAppRuleFlatFeeAndPercentual,
		"ErrCalculationTypePercentual":                constant.ErrCalculationTypePercentual,
		"ErrCalculationTypeFlatFee":                   constant.ErrCalculationTypeFlatFee,
		"ErrFeeFieldsRequired":                        constant.ErrFeeFieldsRequired,
		"ErrCalculationFieldOfFeeRequired":            constant.ErrCalculationFieldOfFeeRequired,
		"ErrReferenceAmountInvalid":                   constant.ErrReferenceAmountInvalid,
		"ErrAppRuleInvalid":                           constant.ErrAppRuleInvalid,
		"ErrCalculationTypeInvalid":                   constant.ErrCalculationTypeInvalid,
		"ErrMaxAmountLessThanMinAmount":               constant.ErrMaxAmountLessThanMinAmount,
		"ErrFilterPackage":                            constant.ErrFilterPackage,
		"ErrPackageRange":                             constant.ErrPackageRange,
		"ErrValidateDistributeTransactionValue":       constant.ErrValidateDistributeTransactionValue,
		"ErrAppRuleMaxBetweenTypes":                   constant.ErrAppRuleMaxBetweenTypes,
		"ErrInvalidSegmentID":                         constant.ErrInvalidSegmentID,
		"ErrInvalidLedgerID":                          constant.ErrInvalidLedgerID,
		"ErrLedgerScopedQueryParameter":               constant.ErrLedgerScopedQueryParameter,
		"ErrConvertToDecimal":                         constant.ErrConvertToDecimal,
		"ErrIsDeductibleFrom":                         constant.ErrIsDeductibleFrom,
		"ErrApplicationRule":                          constant.ErrApplicationRule,
		"ErrCalculationValuePercentage":               constant.ErrCalculationValuePercentage,
		"ErrCalculationValueFlatFee":                  constant.ErrCalculationValueFlatFee,
		"ErrAccessMidaz":                              constant.ErrAccessMidaz,
		"ErrDeductibleCalculationValuePercentage":     constant.ErrDeductibleCalculationValuePercentage,
		"ErrDeductibleCalculationValueFlatFee":        constant.ErrDeductibleCalculationValueFlatFee,
		"ErrInvalidQueryParameterPage":                constant.ErrInvalidQueryParameterPage,
		"ErrBillingPackageNotFound":                   constant.ErrBillingPackageNotFound,
		"ErrInvalidBillingPackageType":                constant.ErrInvalidBillingPackageType,
		"ErrMissingVolumeFields":                      constant.ErrMissingVolumeFields,
		"ErrMissingMaintenanceFields":                 constant.ErrMissingMaintenanceFields,
		"ErrInvalidPricingModel":                      constant.ErrInvalidPricingModel,
		"ErrInvalidPricingTier":                       constant.ErrInvalidPricingTier,
		"ErrBillingRouteOverlap":                      constant.ErrBillingRouteOverlap,
		"ErrTargetAccountNotFound":                    constant.ErrTargetAccountNotFound,
		"ErrBillingCalculationFailed":                 constant.ErrBillingCalculationFailed,
		"ErrNoActiveBillingPackages":                  constant.ErrNoActiveBillingPackages,
		"ErrSegmentResolutionFailed":                  constant.ErrSegmentResolutionFailed,
		"ErrInvalidBillingPeriod":                     constant.ErrInvalidBillingPeriod,
		"ErrInvalidFreeQuota":                         constant.ErrInvalidFreeQuota,
		"ErrInvalidDiscountTier":                      constant.ErrInvalidDiscountTier,
		"ErrInvalidCountMode":                         constant.ErrInvalidCountMode,
		"ErrMidazQueryFailed":                         constant.ErrMidazQueryFailed,
		"ErrInvalidAccountTarget":                     constant.ErrInvalidAccountTarget,
		"ErrInvalidFeeAmount":                         constant.ErrInvalidFeeAmount,
		"ErrMissingSegmentContext":                    constant.ErrMissingSegmentContext,
		"ErrMidazRouteNotFound":                       constant.ErrMidazRouteNotFound,
		"ErrDeductibleFeeExceedsAmount":               constant.ErrDeductibleFeeExceedsAmount,
		"ErrRuleCalculationFieldType":                 constant.ErrRuleCalculationFieldType,
		"ErrParentIDNotFound":                         constant.ErrParentIDNotFound,
		"ErrContextCancelled":                         constant.ErrContextCancelled,
		"ErrPaginationLimitInvalid":                   constant.ErrPaginationLimitInvalid,
		"ErrInvalidSortColumn":                        constant.ErrInvalidSortColumn,
		"ErrInvalidCursor":                            constant.ErrInvalidCursor,
		"ErrCursorWithSortParams":                     constant.ErrCursorWithSortParams,
		"ErrMetadataEntriesExceeded":                  constant.ErrMetadataEntriesExceeded,
		"ErrMetadataKeyInvalidChars":                  constant.ErrMetadataKeyInvalidChars,
		"ErrInvalidDecision":                          constant.ErrInvalidDecision,
		"ErrReasonRequired":                           constant.ErrReasonRequired,
		"ErrInvalidDefaultDecision":                   constant.ErrInvalidDefaultDecision,
		"ErrExpressionSyntax":                         constant.ErrExpressionSyntax,
		"ErrExpressionType":                           constant.ErrExpressionType,
		"ErrExpressionCostExceeded":                   constant.ErrExpressionCostExceeded,
		"ErrExpressionEvaluation":                     constant.ErrExpressionEvaluation,
		"ErrExpressionProgram":                        constant.ErrExpressionProgram,
		"ErrExpressionCostEstimation":                 constant.ErrExpressionCostEstimation,
		"ErrAmountExceedsPrecision":                   constant.ErrAmountExceedsPrecision,
		"ErrRuleNotFound":                             constant.ErrRuleNotFound,
		"ErrRuleNameAlreadyExists":                    constant.ErrRuleNameAlreadyExists,
		"ErrRuleInvalidStatus":                        constant.ErrRuleInvalidStatus,
		"ErrRuleEvaluationFailed":                     constant.ErrRuleEvaluationFailed,
		"ErrExpressionNotModifiable":                  constant.ErrExpressionNotModifiable,
		"ErrRuleNilInput":                             constant.ErrRuleNilInput,
		"ErrRuleNameRequired":                         constant.ErrRuleNameRequired,
		"ErrRuleNameTooLong":                          constant.ErrRuleNameTooLong,
		"ErrRuleExpressionRequired":                   constant.ErrRuleExpressionRequired,
		"ErrRuleExpressionTooLong":                    constant.ErrRuleExpressionTooLong,
		"ErrRuleInvalidAction":                        constant.ErrRuleInvalidAction,
		"ErrRuleInvalidScope":                         constant.ErrRuleInvalidScope,
		"ErrRuleDescriptionTooLong":                   constant.ErrRuleDescriptionTooLong,
		"ErrRuleScopesTooMany":                        constant.ErrRuleScopesTooMany,
		"ErrRuleInvalidTransition":                    constant.ErrRuleInvalidTransition,
		"ErrLimitNotFound":                            constant.ErrLimitNotFound,
		"ErrLimitInvalidStatusChange":                 constant.ErrLimitInvalidStatusChange,
		"ErrLimitInvalidType":                         constant.ErrLimitInvalidType,
		"ErrLimitInvalidMaxAmount":                    constant.ErrLimitInvalidMaxAmount,
		"ErrLimitInvalidCurrency":                     constant.ErrLimitInvalidCurrency,
		"ErrLimitInvalidScope":                        constant.ErrLimitInvalidScope,
		"ErrLimitNameRequired":                        constant.ErrLimitNameRequired,
		"ErrLimitNameTooLong":                         constant.ErrLimitNameTooLong,
		"ErrLimitNameInvalidChars":                    constant.ErrLimitNameInvalidChars,
		"ErrLimitDescriptionInvalidChars":             constant.ErrLimitDescriptionInvalidChars,
		"ErrLimitInvalidID":                           constant.ErrLimitInvalidID,
		"ErrLimitDescriptionTooLong":                  constant.ErrLimitDescriptionTooLong,
		"ErrLimitInvalidStatusFilter":                 constant.ErrLimitInvalidStatusFilter,
		"ErrLimitInvalidTypeFilter":                   constant.ErrLimitInvalidTypeFilter,
		"ErrLimitDeletedAtInvariant":                  constant.ErrLimitDeletedAtInvariant,
		"ErrLimitCheckFailed":                         constant.ErrLimitCheckFailed,
		"ErrLimitNilInput":                            constant.ErrLimitNilInput,
		"ErrLimitImmutableField":                      constant.ErrLimitImmutableField,
		"ErrAuditEventNotFound":                       constant.ErrAuditEventNotFound,
		"ErrInvalidAuditEventFilters":                 constant.ErrInvalidAuditEventFilters,
		"ErrAuditEventInvalidType":                    constant.ErrAuditEventInvalidType,
		"ErrAuditEventInvalidAction":                  constant.ErrAuditEventInvalidAction,
		"ErrAuditEventInvalidResult":                  constant.ErrAuditEventInvalidResult,
		"ErrAuditEventResourceIDRequired":             constant.ErrAuditEventResourceIDRequired,
		"ErrAuditEventInvalidResourceType":            constant.ErrAuditEventInvalidResourceType,
		"ErrAuditEventActorIDRequired":                constant.ErrAuditEventActorIDRequired,
		"ErrAuditEventActorTypeInvalid":               constant.ErrAuditEventActorTypeInvalid,
		"ErrUsageCounterOverflow":                     constant.ErrUsageCounterOverflow,
		"ErrUsageCounterLimitIDRequired":              constant.ErrUsageCounterLimitIDRequired,
		"ErrUsageCounterScopeKeyRequired":             constant.ErrUsageCounterScopeKeyRequired,
		"ErrUsageCounterPeriodKeyRequired":            constant.ErrUsageCounterPeriodKeyRequired,
		"ErrUsageCounterCurrentUsageNegative":         constant.ErrUsageCounterCurrentUsageNegative,
		"ErrUsageCounterIncrementNonNegative":         constant.ErrUsageCounterIncrementNonNegative,
		"ErrUsageCounterNotFound":                     constant.ErrUsageCounterNotFound,
		"ErrUsageCounterExceedsLimit":                 constant.ErrUsageCounterExceedsLimit,
		"ErrUsageCounterDecrementNonNegative":         constant.ErrUsageCounterDecrementNonNegative,
		"ErrCheckLimitsInvalidAmount":                 constant.ErrCheckLimitsInvalidAmount,
		"ErrCheckLimitsInvalidCurrency":               constant.ErrCheckLimitsInvalidCurrency,
		"ErrCheckLimitsUnknownLimitType":              constant.ErrCheckLimitsUnknownLimitType,
		"ErrCheckLimitsInvalidTimestamp":              constant.ErrCheckLimitsInvalidTimestamp,
		"ErrCheckLimitsNilInput":                      constant.ErrCheckLimitsNilInput,
		"ErrCheckLimitsInvalidAccountID":              constant.ErrCheckLimitsInvalidAccountID,
		"ErrCheckLimitsInvalidTransactionType":        constant.ErrCheckLimitsInvalidTransactionType,
		"ErrCheckLimitsInvalidSubType":                constant.ErrCheckLimitsInvalidSubType,
		"ErrCheckLimitsInvalidSegmentID":              constant.ErrCheckLimitsInvalidSegmentID,
		"ErrCheckLimitsInvalidPortfolioID":            constant.ErrCheckLimitsInvalidPortfolioID,
		"ErrCheckLimitsInvalidMerchantID":             constant.ErrCheckLimitsInvalidMerchantID,
		"ErrLimitCheckerNilLimitRepo":                 constant.ErrLimitCheckerNilLimitRepo,
		"ErrLimitCheckerNilUsageCounterRepo":          constant.ErrLimitCheckerNilUsageCounterRepo,
		"ErrLimitCheckerNilClock":                     constant.ErrLimitCheckerNilClock,
		"ErrValidationRequestIDRequired":              constant.ErrValidationRequestIDRequired,
		"ErrValidationInvalidTransactionType":         constant.ErrValidationInvalidTransactionType,
		"ErrValidationAmountNonPositive":              constant.ErrValidationAmountNonPositive,
		"ErrValidationCurrencyRequired":               constant.ErrValidationCurrencyRequired,
		"ErrValidationInvalidCurrency":                constant.ErrValidationInvalidCurrency,
		"ErrValidationTimestampRequired":              constant.ErrValidationTimestampRequired,
		"ErrValidationTimestampFuture":                constant.ErrValidationTimestampFuture,
		"ErrValidationAccountRequired":                constant.ErrValidationAccountRequired,
		"ErrValidationTimestampPast":                  constant.ErrValidationTimestampPast,
		"ErrValidationTimeout":                        constant.ErrValidationTimeout,
		"ErrValidationSegmentIDRequired":              constant.ErrValidationSegmentIDRequired,
		"ErrValidationPortfolioIDRequired":            constant.ErrValidationPortfolioIDRequired,
		"ErrValidationSubTypeTooLong":                 constant.ErrValidationSubTypeTooLong,
		"ErrValidationInvalidAccountType":             constant.ErrValidationInvalidAccountType,
		"ErrValidationInvalidAccountStatus":           constant.ErrValidationInvalidAccountStatus,
		"ErrValidationInvalidMerchantCategory":        constant.ErrValidationInvalidMerchantCategory,
		"ErrValidationInvalidMerchantCountry":         constant.ErrValidationInvalidMerchantCountry,
		"ErrValidationMerchantIDRequired":             constant.ErrValidationMerchantIDRequired,
		"ErrInvalidTransactionValidationFilters":      constant.ErrInvalidTransactionValidationFilters,
		"ErrTransactionValidationNotFound":            constant.ErrTransactionValidationNotFound,
		"ErrListValidationsTimeout":                   constant.ErrListValidationsTimeout,
		"ErrTransactionValidationIDRequired":          constant.ErrTransactionValidationIDRequired,
		"ErrTransactionValidationCreatedAtRequired":   constant.ErrTransactionValidationCreatedAtRequired,
		"ErrRuleCacheWarmUpFailed":                    constant.ErrRuleCacheWarmUpFailed,
		"ErrRuleCacheNotReady":                        constant.ErrRuleCacheNotReady,
		"ErrLimitTimeWindowMismatch":                  constant.ErrLimitTimeWindowMismatch,
		"ErrLimitTimeWindowZeroWidth":                 constant.ErrLimitTimeWindowZeroWidth,
		"ErrTimeOfDayInvalidFormat":                   constant.ErrTimeOfDayInvalidFormat,
		"ErrRuleNameAlreadyExistsInCtx":               constant.ErrRuleNameAlreadyExistsInCtx,
		"ErrLimitNameAlreadyExists":                   constant.ErrLimitNameAlreadyExists,
		"ErrLimitCustomDatesNotAllowed":               constant.ErrLimitCustomDatesNotAllowed,
		"ErrLimitUnknownType":                         constant.ErrLimitUnknownType,
		"ErrLimitCustomPeriodTooLong":                 constant.ErrLimitCustomPeriodTooLong,
		"ErrLimitCustomPeriodExpired":                 constant.ErrLimitCustomPeriodExpired,
		"ErrLimitInvalidCustomStartFormat":            constant.ErrLimitInvalidCustomStartFormat,
		"ErrLimitInvalidCustomEndFormat":              constant.ErrLimitInvalidCustomEndFormat,
		"ErrLimitCustomDatesRequired":                 constant.ErrLimitCustomDatesRequired,
		"ErrLimitCustomDatesOrder":                    constant.ErrLimitCustomDatesOrder,
		"ErrMTConfigRequired":                         constant.ErrMTConfigRequired,
		"ErrMTLoggerRequired":                         constant.ErrMTLoggerRequired,
		"ErrMTURLRequired":                            constant.ErrMTURLRequired,
		"ErrMTURLInvalid":                             constant.ErrMTURLInvalid,
		"ErrMTServiceAPIKeyRequired":                  constant.ErrMTServiceAPIKeyRequired,
		"ErrMTRedisHostRequired":                      constant.ErrMTRedisHostRequired,
		"ErrMTPluginAuthRequired":                     constant.ErrMTPluginAuthRequired,
		"ErrMTAPIKeyOnlyValidationConfl":              constant.ErrMTAPIKeyOnlyValidationConfl,
		"ErrReadyzPgConnectionNotEstablished":         constant.ErrReadyzPgConnectionNotEstablished,
		"ErrReadyzPgConnectionFailed":                 constant.ErrReadyzPgConnectionFailed,
		"ErrReadyzPgPingFailed":                       constant.ErrReadyzPgPingFailed,
		"ErrReadyzDependenciesUnhealthy":              constant.ErrReadyzDependenciesUnhealthy,
		"ErrReadyzCacheNotReady":                      constant.ErrReadyzCacheNotReady,
		"ErrReadyzCacheStale":                         constant.ErrReadyzCacheStale,
		"ErrSupervisorShuttingDown":                   constant.ErrSupervisorShuttingDown,
		"ErrTenantCapReached":                         constant.ErrTenantCapReached,
		"ErrSupervisorNilRuleCache":                   constant.ErrSupervisorNilRuleCache,
		"ErrSupervisorNilSyncRepo":                    constant.ErrSupervisorNilSyncRepo,
		"ErrSupervisorNilUsageRepo":                   constant.ErrSupervisorNilUsageRepo,
		"ErrSupervisorNilCompiler":                    constant.ErrSupervisorNilCompiler,
		"ErrSupervisorNilLogger":                      constant.ErrSupervisorNilLogger,
		"ErrSupervisorNilReaperRepo":                  constant.ErrSupervisorNilReaperRepo,
		"ErrSupervisorNilReaperAuditor":               constant.ErrSupervisorNilReaperAuditor,
		"ErrUnauthorizedMissingSub":                   constant.ErrUnauthorizedMissingSub,
		"ErrReservationLimitIDRequired":               constant.ErrReservationLimitIDRequired,
		"ErrReservationTransactionIDReq":              constant.ErrReservationTransactionIDReq,
		"ErrReservationScopeKeyRequired":              constant.ErrReservationScopeKeyRequired,
		"ErrReservationPeriodKeyRequired":             constant.ErrReservationPeriodKeyRequired,
		"ErrReservationAmountInvalid":                 constant.ErrReservationAmountInvalid,
		"ErrReservationInvalidStatus":                 constant.ErrReservationInvalidStatus,
		"ErrReservationExpiresAtRequired":             constant.ErrReservationExpiresAtRequired,
		"ErrReservationNotFound":                      constant.ErrReservationNotFound,
		"ErrReservationAlreadyTerminal":               constant.ErrReservationAlreadyTerminal,
		"ErrRouteNotFound":                            constant.ErrRouteNotFound,
		"ErrMethodNotAllowed":                         constant.ErrMethodNotAllowed,
		"ErrPendingTransactionLocked":                 constant.ErrPendingTransactionLocked,
		"ErrReservationTenantRequired":                constant.ErrReservationTenantRequired,
		"ErrInstrumentLedgerReferenceNotFound":        constant.ErrInstrumentLedgerReferenceNotFound,
		"ErrInstrumentAccountReferenceNotFound":       constant.ErrInstrumentAccountReferenceNotFound,
		"ErrSkipNotPermitted":                         constant.ErrSkipNotPermitted,
		"ErrHolderRequired":                           constant.ErrHolderRequired,
		"ErrHolderNotFound":                           constant.ErrHolderNotFound,
		"ErrInstrumentNotFound":                       constant.ErrInstrumentNotFound,
		"ErrDocumentAssociationError":                 constant.ErrDocumentAssociationError,
		"ErrAccountAlreadyAssociated":                 constant.ErrAccountAlreadyAssociated,
		"ErrHolderHasInstruments":                     constant.ErrHolderHasInstruments,
		"ErrMetadataQueryInvalidFormat":               constant.ErrMetadataQueryInvalidFormat,
		"ErrMetadataQueryInvalidKey":                  constant.ErrMetadataQueryInvalidKey,
		"ErrMetadataQueryContainsOperator":            constant.ErrMetadataQueryContainsOperator,
		"ErrInvalidHeaderValue":                       constant.ErrInvalidHeaderValue,
		"ErrInstrumentClosingDateBeforeCreation":      constant.ErrInstrumentClosingDateBeforeCreation,
		"ErrRelatedPartyNotFound":                     constant.ErrRelatedPartyNotFound,
		"ErrInvalidRelatedPartyRole":                  constant.ErrInvalidRelatedPartyRole,
		"ErrRelatedPartyDocumentRequired":             constant.ErrRelatedPartyDocumentRequired,
		"ErrRelatedPartyNameRequired":                 constant.ErrRelatedPartyNameRequired,
		"ErrRelatedPartyStartDateRequired":            constant.ErrRelatedPartyStartDateRequired,
		"ErrRelatedPartyEndDateInvalid":               constant.ErrRelatedPartyEndDateInvalid,
		"ErrHolderHasAccounts":                        constant.ErrHolderHasAccounts,
		"ErrKeysetNotFound":                           constant.ErrKeysetNotFound,
		"ErrKeysetAlreadyExists":                      constant.ErrKeysetAlreadyExists,
		"ErrKeysetRevisionConflict":                   constant.ErrKeysetRevisionConflict,
		"ErrRegistryNotFound":                         constant.ErrRegistryNotFound,
		"ErrRegistryAlreadyExists":                    constant.ErrRegistryAlreadyExists,
		"ErrRegistryRevisionConflict":                 constant.ErrRegistryRevisionConflict,
		"ErrOrganizationEncryptionFailed":             constant.ErrOrganizationEncryptionFailed,
		"ErrProvisioningFailed":                       constant.ErrProvisioningFailed,
		"ErrAuditEventRequired":                       constant.ErrAuditEventRequired,
		"ErrAuditWriteFailed":                         constant.ErrAuditWriteFailed,
		"ErrReservedTenantID":                         constant.ErrReservedTenantID,
		"ErrTransactionScopeMismatch":                 constant.ErrTransactionScopeMismatch,
		"ErrOverdraftRouteNotConfigured":              constant.ErrOverdraftRouteNotConfigured,
		"ErrReadyzRedisConnectionNotEstablished":      constant.ErrReadyzRedisConnectionNotEstablished,
		"ErrReadyzRedisPingFailed":                    constant.ErrReadyzRedisPingFailed,
		"ErrReadyzStreamingUnhealthy":                 constant.ErrReadyzStreamingUnhealthy,
		"ErrReadyzTenantManagerUnavailable":           constant.ErrReadyzTenantManagerUnavailable,
	}
}

// sentinelRegistryPaths resolves every non-test source file of pkg/constant relative to THIS
// file, so the registry is found whatever directory `go test` was invoked from.
//
// The whole package is swept, not just errors.go: nothing stops a sentinel from being declared
// in a sibling file of the same package, and a walk pinned to one filename would not see it.
func sentinelRegistryPaths(t *testing.T) []string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller must resolve this test file to locate the sentinel registry")

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(thisFile), "..", "..", "constant", "*.go"))
	require.NoError(t, err, "the sentinel registry glob must resolve")

	paths := make([]string, 0, len(matches))

	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		paths = append(paths, path)
	}

	require.NotEmpty(t, paths, "the sentinel registry package must contain at least one non-test source file")

	sort.Strings(paths)

	return paths
}

// declaredSentinelNames is the AUTHORITATIVE sentinel set: the name of every package-level
// Err* var in pkg/constant, read out of the source with go/ast. Deriving it from the registry
// rather than restating it is what makes TestGolden_SentinelInventoryComplete an actual guard —
// comparing two hand-written literals would observe the registry not at all.
//
// Candidates are selected by the errors.New INITIALISER, not by the name, because the initialiser
// is what makes a var a sentinel. Selecting on the `Err` prefix would silently skip a sentinel
// named without it — the sweep would never drive it, and the two-way diff would stay green.
//
// Both halves of the convention then have to fail loudly, since the walk is the only thing that
// decides what gets swept: a prefixed var built some other way (fmt.Errorf, a helper) is a form
// this parser cannot classify, and an errors.New var without the prefix breaks the naming
// invariant every sentinel in the registry holds.
func declaredSentinelNames(t *testing.T) []string {
	t.Helper()

	fset := token.NewFileSet()

	var names []string

	for _, path := range sentinelRegistryPaths(t) {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		require.NoErrorf(t, err, "the sentinel registry file %s must parse", path)

		for _, decl := range file.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen || gen.Tok != token.VAR {
				continue
			}

			for _, spec := range gen.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue {
					continue
				}

				for i, name := range value.Names {
					hasErrPrefix := strings.HasPrefix(name.Name, "Err")
					isErrorsNew := i < len(value.Values) && isErrorsNewCall(value.Values[i])

					if !hasErrPrefix && !isErrorsNew {
						continue
					}

					require.Truef(t, isErrorsNew,
						"%s declares %s with the Err prefix but no errors.New initialiser: this walk cannot classify it, so it would escape the golden sweep — declare it with errors.New or teach this parser the new form",
						filepath.Base(path), name.Name)

					require.Truef(t, hasErrPrefix,
						"%s declares the errors.New sentinel %s without the Err prefix, which every sentinel in this registry carries — rename it so the registry stays greppable by that prefix",
						filepath.Base(path), name.Name)

					names = append(names, name.Name)
				}
			}
		}
	}

	// An AST walk that silently stops matching would make the diff below pass
	// vacuously in the one direction that matters, so refuse an empty result.
	require.NotEmpty(t, names, "the AST walk found no sentinels — the registry shape changed and this parser no longer matches it")

	sort.Strings(names)

	return names
}

// isErrorsNewCall reports whether expr is a call to errors.New.
func isErrorsNewCall(expr ast.Expr) bool {
	call, isCall := expr.(*ast.CallExpr)
	if !isCall {
		return false
	}

	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != "New" {
		return false
	}

	pkgIdent, isIdent := sel.X.(*ast.Ident)

	return isIdent && pkgIdent.Name == "errors"
}

// missingNames returns every element of want that got does not contain.
func missingNames(want, got []string) []string {
	present := make(map[string]struct{}, len(got))
	for _, name := range got {
		present[name] = struct{}{}
	}

	var missing []string

	for _, name := range want {
		if _, ok := present[name]; !ok {
			missing = append(missing, name)
		}
	}

	return missing
}

// TestGolden_SentinelInventoryComplete guards allSentinels against drift by diffing it, name for
// name, against the registry parsed out of pkg/constant: a sentinel added there and not here (or
// removed there and left here) fails with the offending names in the diff.
//
// The two diffs are a SET EQUALITY, which fixes the size as a consequence — so no separate count
// is asserted. A hardcoded total would only make every sentinel addition a three-place edit
// while catching nothing the diffs miss, and a parser regression returning a subset is already
// caught by the second diff plus the non-empty guard in declaredSentinelNames.
func TestGolden_SentinelInventoryComplete(t *testing.T) {
	t.Parallel()

	declared := declaredSentinelNames(t)
	inventory := allSentinels()

	inventoried := make([]string, 0, len(inventory))
	for name := range inventory {
		inventoried = append(inventoried, name)
	}

	sort.Strings(inventoried)

	// Report the deltas rather than asserting slice equality: a whole-registry diff buries
	// the one name that actually moved.
	assert.Empty(t, missingNames(declared, inventoried),
		"sentinels declared in pkg/constant but missing from allSentinels() — the sweep never drives them, so their (code, status) tuple is unpinned")
	assert.Empty(t, missingNames(inventoried, declared),
		"names in allSentinels() that pkg/constant no longer declares — drop them from the inventory")

	// A key/value mismatch (a name mapped to the wrong constant) survives the name
	// diff above but leaves one sentinel unswept and another swept twice. Distinct
	// sentinels have distinct codes, so a duplicated value is the observable symptom.
	byCode := make(map[string]string, len(inventory))

	for name, sentinel := range inventory {
		if prev, dup := byCode[sentinel.Error()]; dup {
			t.Errorf("inventory entries %q and %q map to the same sentinel %q — one of the two keys names the wrong constant", prev, name, sentinel.Error())
		}

		byCode[sentinel.Error()] = name
	}
}

// TestGolden_BusinessErrorCodeStatus is the full sweep over the first of the three
// routes a code can take to the wire, the ValidateBusinessError one: every
// sentinel, driven through the REAL WithError dispatcher, must emit the (status,
// code) the frozen classifier derives. Mapped sentinels classify by their typed
// error; unmapped sentinels (the ~18 defined-but-unmapped) fall through
// ValidateBusinessError (returns the bare sentinel) and then through WithError to
// 500 / code 0046 — which is exactly what the classifier's fallthrough returns.
func TestGolden_BusinessErrorCodeStatus(t *testing.T) {
	t.Parallel()

	inventory := allSentinels()

	names := make([]string, 0, len(inventory))
	for name := range inventory {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		sentinel := inventory[name]
		validated := pkg.ValidateBusinessError(sentinel, "GoldenEntity")
		wantStatus, wantCode := classifyStatusOf(t, validated)

		t.Run(name+"_"+wantCode+"_"+strconv.Itoa(wantStatus), func(t *testing.T) {
			t.Parallel()

			gotStatus, gotCode := driveWithError(t, validated)
			assert.Equal(t, wantStatus, gotStatus, "MONEY-PATH: HTTP status for sentinel %q", sentinel.Error())
			assert.Equal(t, wantCode, gotCode, "MONEY-PATH: body[code] for sentinel %q", sentinel.Error())
		})
	}
}

// TestGolden_HelperPathCodeStatus covers the second route to the wire: the
// sentinels reached not through ValidateBusinessError but through the three helper
// constructors. The sweep above drives those sentinels too, but only as unmapped
// fallthroughs; it never builds the typed error the helper produces, so this is the
// only place the constructors' own (code, status) tuple is pinned. Plus the two
// named-case checks (FailedPreconditionError->500, fallthrough->500/0046).
// Schema drift must reach the wire as 503, not 500: the schema is applied out of
// band, so the same request succeeds once the migration runner reaches the
// database. A 5xx that reads as permanent would send clients to support instead
// of to a retry.
func TestGolden_SchemaMigrationPendingIsRetryable(t *testing.T) {
	t.Parallel()

	err := pkg.ValidateBusinessError(constant.ErrSchemaMigrationPending, "GoldenEntity")

	status, code := driveWithError(t, err)

	assert.Equal(t, fiber.StatusServiceUnavailable, status)
	assert.Equal(t, constant.ErrSchemaMigrationPending.Error(), code)
}

func TestGolden_HelperPathCodeStatus(t *testing.T) {
	t.Parallel()

	dummyFields := map[string]string{"field": "message"}
	dummyUnknown := map[string]any{"extra": "value"}

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			// ValidateInternalError -> InternalServerError -> 500 / 0046.
			name:       "internal_error_0046_500",
			err:        pkg.ValidateInternalError(errors.New("boom"), ""),
			wantStatus: fiber.StatusInternalServerError,
			wantCode:   constant.ErrInternalServer.Error(), // 0046
		},
		{
			// ValidateBadRequestFieldsError requiredFields branch -> 0009 / 400.
			name:       "missing_fields_0009_400",
			err:        pkg.ValidateBadRequestFieldsError(dummyFields, nil, "", nil),
			wantStatus: fiber.StatusBadRequest,
			wantCode:   constant.ErrMissingFieldsInRequest.Error(), // 0009
		},
		{
			// ValidateBadRequestFieldsError knownInvalidFields branch -> 0047 / 400.
			name:       "bad_request_0047_400",
			err:        pkg.ValidateBadRequestFieldsError(nil, dummyFields, "", nil),
			wantStatus: fiber.StatusBadRequest,
			wantCode:   constant.ErrBadRequest.Error(), // 0047
		},
		{
			// ValidateBadRequestFieldsError unknownFields branch -> 0053 / 400.
			name:       "unexpected_fields_0053_400",
			err:        pkg.ValidateBadRequestFieldsError(nil, nil, "", dummyUnknown),
			wantStatus: fiber.StatusBadRequest,
			wantCode:   constant.ErrUnexpectedFieldsInTheRequest.Error(), // 0053
		},
		{
			// FailedPreconditionError -> 500, NOT the 412 the name suggests: it
			// renders through the internal-server arm like InternalServerError.
			name: "failed_precondition_500_not_412",
			err: pkg.FailedPreconditionError{
				Code:    constant.ErrJWKFetch.Error(), // 0045
				Title:   "JWK Fetch Error",
				Message: "JWK keys could not be fetched",
			},
			wantStatus: fiber.StatusInternalServerError,
			wantCode:   constant.ErrJWKFetch.Error(),
		},
		{
			// Fallthrough: a plain error matches no arm of the cascade, so the
			// dispatcher's unclassified-error fallback answers 500 / code 0046.
			// The sweep reaches this arm too (via the unmapped sentinels) but
			// derives its expectation from the same classifier that models it, so
			// it would agree with itself if the fallback moved. Spelling 500/0046
			// out literally here is what anchors that end of the classifier.
			name:       "fallthrough_plain_error_500_0046",
			err:        errors.New("some unmapped raw error"),
			wantStatus: fiber.StatusInternalServerError,
			wantCode:   constant.ErrInternalServer.Error(), // 0046
		},
		// libCommons.Response arm (classifyForProblem/problem.go:94-105): the
		// hottest money-path branch. lib-commons emits commons.Response for
		// balance/transaction rejections; these four codes must keep their status
		// when they arrive wrapped as a Response, NOT their table default (400).
		// Pinned explicitly because classifyStatusOf has no libCommons arm, so the
		// self-generating sweep never reaches this branch.
		{
			// Insufficient funds -> 422 (money-path rejection). NOT 400.
			name:       "libcommons_insufficient_funds_0018_422",
			err:        libCommons.Response{Code: libConstants.ErrInsufficientFunds.Error(), Message: "insufficient funds"},
			wantStatus: fiber.StatusUnprocessableEntity,
			wantCode:   libConstants.ErrInsufficientFunds.Error(), // 0018
		},
		{
			// Account ineligibility -> 422 (money-path rejection). NOT 400.
			name:       "libcommons_account_ineligibility_0019_422",
			err:        libCommons.Response{Code: libConstants.ErrAccountIneligibility.Error(), Message: "account ineligible"},
			wantStatus: fiber.StatusUnprocessableEntity,
			wantCode:   libConstants.ErrAccountIneligibility.Error(), // 0019
		},
		{
			// Asset code not found -> 404. NOT 400.
			name:       "libcommons_asset_code_not_found_0034_404",
			err:        libCommons.Response{Code: libConstants.ErrAssetCodeNotFound.Error(), Message: "asset code not found"},
			wantStatus: fiber.StatusNotFound,
			wantCode:   libConstants.ErrAssetCodeNotFound.Error(), // 0034
		},
		{
			// Int64 overflow -> 422. NOT 400, and NOT 500: the caller sent values whose
			// sum exceeds int64, which is the same class as insufficient funds. The pkg
			// registry entry for 0097 carries the same status, so the code answers with
			// one status whichever layer produced it.
			name:       "libcommons_overflow_int64_0097_422",
			err:        libCommons.Response{Code: libConstants.ErrOverFlowInt64.Error(), Message: "overflow"},
			wantStatus: fiber.StatusUnprocessableEntity,
			wantCode:   libConstants.ErrOverFlowInt64.Error(), // 0097
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotStatus, gotCode := driveWithError(t, tc.err)
			assert.Equal(t, tc.wantStatus, gotStatus, "MONEY-PATH: HTTP status")
			assert.Equal(t, tc.wantCode, gotCode, "MONEY-PATH: body[code]")
		})
	}
}

// TestGolden_UnmarshallingStatusInCode locks the ResponseError status-in-Code
// quirk, the one helper-constructor path whose status is not an HTTP status at all:
// ValidateUnmarshallingError produces a ResponseError whose Code is "0094", and
// JSONResponseError writes the status as strconv.Atoi("0094") = 94. Go's net/http
// client (app.Test) rejects status 94 as malformed (< 100), so the raw fiber
// response is inspected directly instead of through an HTTP roundtrip. If the quirk
// ever changes (e.g. 0094 mapped to a real HTTP status), the raw status here changes
// and this test goes RED.
func TestGolden_UnmarshallingStatusInCode(t *testing.T) {
	t.Parallel()

	respErr := pkg.ValidateUnmarshallingError(errors.New("bad json"))

	var captured pkg.ResponseError
	require.True(t, errors.As(respErr, &captured), "ValidateUnmarshallingError must return a ResponseError")

	// The quirk: status IS the numeric Code. Lock code + derived status.
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), captured.Code, "MONEY-PATH: 0094 code")

	gotStatus, convErr := strconv.Atoi(captured.Code)
	require.NoError(t, convErr)
	assert.Equal(t, 94, gotStatus, "MONEY-PATH: 0094 status-in-Code quirk = 94 (response.go:124)")

	// And confirm WithError actually routes ResponseError to JSONResponseError,
	// i.e. the dispatcher writes that status onto the fiber response. Read the raw
	// fasthttp status (app.Test would reject 94 as malformed HTTP).
	app := fiber.New(fiber.Config{
		ErrorHandler: CanonicalFiberErrorHandler,
	})

	var writtenStatus int

	var writtenBody string

	app.Get("/probe", func(c fiber.Ctx) error {
		dispatchErr := WithError(c, respErr)
		writtenStatus = c.Response().StatusCode()
		writtenBody = string(c.Response().Body())

		return dispatchErr
	})

	// Drive the handler; ignore the transport-level parse error on status 94.
	_, _ = app.Test(httptest.NewRequest(fiber.MethodGet, "/probe", nil))

	assert.Equal(t, 94, writtenStatus, "MONEY-PATH: WithError writes status 94 for ResponseError 0094")

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(writtenBody), &body), "body must be JSON, got: %s", writtenBody)

	codeVal, _ := body["code"].(string)
	assert.Equal(t, constant.ErrInvalidRequestBody.Error(), codeVal, "MONEY-PATH: 0094 body[code]")
}

// TestGolden_ExplicitStatusArms covers the third route to the wire, and the only
// AMBIGUOUS one: the two codes whose status comes from renderCanonical (an explicit
// status), NOT from the type->status cascade. 0485 -> 405 and 0143 -> 413 here,
// while the very same codes are mapped to ValidationError, so the sweep above sees
// them at 400. Nothing else observes the explicit-status side, so without these two
// cases both arms could collapse into their 400 table status unnoticed.
func TestGolden_ExplicitStatusArms(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		fiberCode  int
		wantStatus int
		wantCode   string
	}{
		{
			name:       "method_not_allowed_0485_405",
			fiberCode:  fiber.StatusMethodNotAllowed,
			wantStatus: fiber.StatusMethodNotAllowed,         // 405
			wantCode:   constant.ErrMethodNotAllowed.Error(), // 0485
		},
		{
			name:       "payload_too_large_0143_413",
			fiberCode:  fiber.StatusRequestEntityTooLarge,
			wantStatus: fiber.StatusRequestEntityTooLarge,   // 413
			wantCode:   constant.ErrPayloadTooLarge.Error(), // 0143
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{
				ErrorHandler: CanonicalFiberErrorHandler,
			})
			app.Get("/probe", func(c fiber.Ctx) error {
				return fiber.NewError(tc.fiberCode, "escaped error")
			})

			resp, testErr := app.Test(httptest.NewRequest(fiber.MethodGet, "/probe", nil))
			require.NoError(t, testErr)

			defer func() { _ = resp.Body.Close() }()

			b, readErr := io.ReadAll(resp.Body)
			require.NoError(t, readErr)

			var body map[string]any
			require.NoError(t, json.Unmarshal(b, &body), "body must be JSON, got: %s", string(b))

			codeVal, _ := body["code"].(string)

			assert.Equal(t, tc.wantStatus, resp.StatusCode, "MONEY-PATH: explicit-status HTTP status")
			assert.Equal(t, tc.wantCode, codeVal, "MONEY-PATH: explicit-status body[code]")
		})
	}
}
