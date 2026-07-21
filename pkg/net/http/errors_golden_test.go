// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file is the MONEY-PATH golden net for the (code, HTTP status) table.
//
// It drives every error through the REAL dispatcher (WithError via fiber, or
// CanonicalFiberErrorHandler for the two explicit-status arms) and asserts ONLY
// resp.StatusCode and body["code"] — the two money-path invariants that must
// survive byte-for-byte across the WithError -> problem.MapError swap.
//
// The expected (status, code) for each case is derived by classifyStatusOf: the
// SAME errors.As cascade, in the SAME declaration order, that WithError walks
// (r3-moneypath-swap-spec.md §1). That classifier IS the frozen statusOf the
// production swap will have to match. Because the sweep re-derives the expected
// value from the classifier and compares it to what the live dispatcher emits,
// the test is self-generating and drift-proof: add a code, it is swept; change a
// code's type/status, this test goes RED.
//
// ponytail: one classifier, swept over every sentinel — the smallest thing that
// fails if a code or status drifts.

// classifyStatusOf reproduces §1 VERBATIM: the errors.As cascade of WithError,
// first match wins, returning the (HTTP status, code) that arm would emit. On no
// match it returns the fallthrough (500, "0046") — matching WithError's final
// ValidateInternalError(err, "") arm (errors.go:110).
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
		return fiber.StatusInternalServerError, e.Code // NOT 412 — §1 row 11
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
		DisableStartupMessage: true,
		ErrorHandler:          CanonicalFiberErrorHandler,
	})
	app.Get("/probe", func(c *fiber.Ctx) error { return WithError(c, err) })

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

// allSentinels is every constant.Err* sentinel declared in pkg/constant/errors.go.
// Kept as a literal slice because Go cannot reflect over package-level vars; the
// count is guarded by TestGolden_SentinelInventoryComplete so a new sentinel that
// is not added here fails CI rather than silently escaping the sweep.
func allSentinels() []error {
	return []error{
		constant.ErrDuplicateLedger,
		constant.ErrLedgerNameConflict,
		constant.ErrAssetNameOrCodeDuplicate,
		constant.ErrCodeUppercaseRequirement,
		constant.ErrCurrencyCodeStandardCompliance,
		constant.ErrUnmodifiableField,
		constant.ErrEntityNotFound,
		constant.ErrActionNotPermitted,
		constant.ErrMissingFieldsInRequest,
		constant.ErrAccountTypeImmutable,
		constant.ErrInactiveAccountType,
		constant.ErrAccountBalanceDeletion,
		constant.ErrResourceAlreadyDeleted,
		constant.ErrSegmentIDInactive,
		constant.ErrDuplicateSegmentName,
		constant.ErrInvalidScriptFormat,
		constant.ErrInsufficientFunds,
		constant.ErrAccountIneligibility,
		constant.ErrAliasUnavailability,
		constant.ErrParentTransactionIDNotFound,
		constant.ErrImmutableField,
		constant.ErrTransactionTimingRestriction,
		constant.ErrAccountStatusTransactionRestriction,
		constant.ErrInsufficientAccountBalance,
		constant.ErrTransactionMethodRestriction,
		constant.ErrDuplicateTransactionTemplateCode,
		constant.ErrDuplicateAssetPair,
		constant.ErrInvalidParentAccountID,
		constant.ErrMismatchedAssetCode,
		constant.ErrChartTypeNotFound,
		constant.ErrInvalidCountryCode,
		constant.ErrInvalidCodeFormat,
		constant.ErrAssetCodeNotFound,
		constant.ErrPortfolioIDNotFound,
		constant.ErrSegmentIDNotFound,
		constant.ErrLedgerIDNotFound,
		constant.ErrOrganizationIDNotFound,
		constant.ErrParentOrganizationIDNotFound,
		constant.ErrInvalidType,
		constant.ErrTokenMissing,
		constant.ErrInvalidToken,
		constant.ErrInsufficientPrivileges,
		constant.ErrPermissionEnforcement,
		constant.ErrJWKFetch,
		constant.ErrInternalServer,
		constant.ErrBadRequest,
		constant.ErrInvalidDSLFileFormat,
		constant.ErrEmptyDSLFile,
		constant.ErrMetadataKeyLengthExceeded,
		constant.ErrMetadataValueLengthExceeded,
		constant.ErrAccountIDNotFound,
		constant.ErrUnexpectedFieldsInTheRequest,
		constant.ErrIDsNotFoundForAccounts,
		constant.ErrAssetIDNotFound,
		constant.ErrNoAssetsFound,
		constant.ErrNoSegmentsFound,
		constant.ErrNoPortfoliosFound,
		constant.ErrNoOrganizationsFound,
		constant.ErrNoLedgersFound,
		constant.ErrBalanceUpdateFailed,
		constant.ErrNoAccountIDsProvided,
		constant.ErrFailedToRetrieveAccountsByAliases,
		constant.ErrNoAccountsFound,
		constant.ErrInvalidPathParameter,
		constant.ErrInvalidAccountType,
		constant.ErrInvalidMetadataNesting,
		constant.ErrOperationIDNotFound,
		constant.ErrNoOperationsFound,
		constant.ErrTransactionIDNotFound,
		constant.ErrNoTransactionsFound,
		constant.ErrInvalidTransactionType,
		constant.ErrTransactionValueMismatch,
		constant.ErrForbiddenExternalAccountManipulation,
		constant.ErrAuditRecordNotRetrieved,
		constant.ErrAuditTreeRecordNotFound,
		constant.ErrInvalidDateFormat,
		constant.ErrInvalidFinalDate,
		constant.ErrDateRangeExceedsLimit,
		constant.ErrPaginationLimitExceeded,
		constant.ErrInvalidSortOrder,
		constant.ErrInvalidQueryParameter,
		constant.ErrInvalidDateRange,
		constant.ErrIdempotencyKey,
		constant.ErrAccountAliasNotFound,
		constant.ErrLockVersionAccountBalance,
		constant.ErrTransactionIDHasAlreadyParentTransaction,
		constant.ErrTransactionIDIsAlreadyARevert,
		constant.ErrTransactionCantRevert,
		constant.ErrTransactionAmbiguous,
		constant.ErrParentIDSameID,
		constant.ErrNoBalancesFound,
		constant.ErrBalancesCantBeDeleted,
		constant.ErrInvalidRequestBody,
		constant.ErrMessageBrokerUnavailable,
		constant.ErrAccountAliasInvalid,
		constant.ErrOverFlowInt64,
		constant.ErrOnHoldExternalAccount,
		constant.ErrCommitTransactionNotPending,
		constant.ErrOperationRouteTitleAlreadyExists,
		constant.ErrOperationRouteNotFound,
		constant.ErrNoOperationRoutesFound,
		constant.ErrInvalidOperationRouteType,
		constant.ErrMissingOperationRoutes,
		constant.ErrTransactionRouteNotFound,
		constant.ErrNoTransactionRoutesFound,
		constant.ErrOperationRouteLinkedToTransactionRoutes,
		constant.ErrDuplicateAccountTypeKeyValue,
		constant.ErrAccountTypeNotFound,
		constant.ErrNoAccountTypesFound,
		constant.ErrInvalidAccountRuleType,
		constant.ErrInvalidAccountRuleValue,
		constant.ErrCorruptedAccountRule,
		constant.ErrTransactionRouteNotInformed,
		constant.ErrInvalidTransactionRouteID,
		constant.ErrAccountingRouteCountMismatch,
		constant.ErrAccountingRouteNotFound,
		constant.ErrAccountingAliasValidationFailed,
		constant.ErrAccountingAccountTypeValidationFailed,
		constant.ErrInvalidAccountTypeKeyValue,
		constant.ErrInvalidFutureTransactionDate,
		constant.ErrInvalidPendingFutureTransactionDate,
		constant.ErrDuplicatedAliasKeyValue,
		constant.ErrAdditionalBalanceNotAllowed,
		constant.ErrInvalidTransactionNonPositiveValue,
		constant.ErrDefaultBalanceNotFound,
		constant.ErrAccountCreationFailed,
		constant.ErrTransactionBackupCacheFailed,
		constant.ErrTransactionBackupCacheMarshalFailed,
		constant.ErrInvalidDatetimeFormat,
		constant.ErrMetadataIndexAlreadyExists,
		constant.ErrMetadataIndexNotFound,
		constant.ErrMetadataIndexInvalidKey,
		constant.ErrMetadataIndexLimitExceeded,
		constant.ErrMetadataIndexCreationFailed,
		constant.ErrMetadataIndexDeletionForbidden,
		constant.ErrInvalidEntityName,
		constant.ErrTransactionBackupCacheRetrievalFailed,
		constant.ErrInvalidTimestamp,
		constant.ErrNoBalanceDataAtTimestamp,
		constant.ErrMissingRequiredQueryParameter,
		constant.ErrPayloadTooLarge,
		constant.ErrRequestHeaderFieldsTooLarge,
		constant.ErrJSONNestingDepthExceeded,
		constant.ErrJSONKeyCountExceeded,
		constant.ErrTenantNotProvisioned,
		constant.ErrUnknownSettingsField,
		constant.ErrInvalidSettingsFieldType,
		constant.ErrSettingsRootLevelField,
		constant.ErrRouteNotBidirectional,
		constant.ErrMissingCounterpart,
		constant.ErrDirectionRouteMismatch,
		constant.ErrNoSourceForAction,
		constant.ErrNoDestinationForAction,
		constant.ErrInvalidRouteAction,
		constant.ErrNoRoutesForAction,
		constant.ErrTooManyOperationRoutes,
		constant.ErrTenantServiceSuspended,
		constant.ErrTenantNotFound,
		constant.ErrTenantServiceUnavailable,
		constant.ErrScenarioNotAllowedForDirection,
		constant.ErrReserveGroupIncomplete,
		constant.ErrDirectScenarioRequired,
		constant.ErrRevertOnlyBidirectional,
		constant.ErrAccountingEntryFieldRequired,
		constant.ErrOverdraftLimitExceeded,
		constant.ErrDirectOperationOnInternalBalance,
		constant.ErrDeletionOfInternalBalance,
		constant.ErrReservedBalanceKey,
		constant.ErrInvalidBalanceDirection,
		constant.ErrInvalidBalanceSettings,
		constant.ErrOverdraftLimitBelowUsage,
		constant.ErrStaleBalanceVersion,
		constant.ErrUpdateOfInternalBalance,
		constant.ErrInvalidSettingsFieldValue,
		constant.ErrTransactionReservationDenied,
		constant.ErrTransactionReservationUnavailable,
		constant.ErrFeeCalculationFieldType,
		constant.ErrPriorityInvalid,
		constant.ErrFindAccountOnMidaz,
		constant.ErrMinAmountGreaterThanMaxAmount,
		constant.ErrNothingToUpdate,
		constant.ErrDuplicatePackage,
		constant.ErrFeeInvalidHeaderParameter,
		constant.ErrCalculateFee,
		constant.ErrCalculationRequired,
		constant.ErrPriorityOne,
		constant.ErrAppRuleFlatFeeAndPercentual,
		constant.ErrCalculationTypePercentual,
		constant.ErrCalculationTypeFlatFee,
		constant.ErrFeeFieldsRequired,
		constant.ErrCalculationFieldOfFeeRequired,
		constant.ErrReferenceAmountInvalid,
		constant.ErrAppRuleInvalid,
		constant.ErrCalculationTypeInvalid,
		constant.ErrMaxAmountLessThanMinAmount,
		constant.ErrFilterPackage,
		constant.ErrPackageRange,
		constant.ErrValidateDistributeTransactionValue,
		constant.ErrAppRuleMaxBetweenTypes,
		constant.ErrInvalidSegmentID,
		constant.ErrInvalidLedgerID,
		constant.ErrConvertToDecimal,
		constant.ErrIsDeductibleFrom,
		constant.ErrApplicationRule,
		constant.ErrCalculationValuePercentage,
		constant.ErrCalculationValueFlatFee,
		constant.ErrAccessMidaz,
		constant.ErrDeductibleCalculationValuePercentage,
		constant.ErrDeductibleCalculationValueFlatFee,
		constant.ErrInvalidQueryParameterPage,
		constant.ErrBillingPackageNotFound,
		constant.ErrInvalidBillingPackageType,
		constant.ErrMissingVolumeFields,
		constant.ErrMissingMaintenanceFields,
		constant.ErrInvalidPricingModel,
		constant.ErrInvalidPricingTier,
		constant.ErrBillingRouteOverlap,
		constant.ErrTargetAccountNotFound,
		constant.ErrBillingCalculationFailed,
		constant.ErrNoActiveBillingPackages,
		constant.ErrSegmentResolutionFailed,
		constant.ErrInvalidBillingPeriod,
		constant.ErrInvalidFreeQuota,
		constant.ErrInvalidDiscountTier,
		constant.ErrInvalidCountMode,
		constant.ErrMidazQueryFailed,
		constant.ErrInvalidAccountTarget,
		constant.ErrInvalidFeeAmount,
		constant.ErrMissingSegmentContext,
		constant.ErrMidazRouteNotFound,
		constant.ErrDeductibleFeeExceedsAmount,
		constant.ErrRuleCalculationFieldType,
		constant.ErrParentIDNotFound,
		constant.ErrContextCancelled,
		constant.ErrPaginationLimitInvalid,
		constant.ErrInvalidSortColumn,
		constant.ErrInvalidCursor,
		constant.ErrCursorWithSortParams,
		constant.ErrMetadataEntriesExceeded,
		constant.ErrMetadataKeyInvalidChars,
		constant.ErrInvalidDecision,
		constant.ErrReasonRequired,
		constant.ErrInvalidDefaultDecision,
		constant.ErrExpressionSyntax,
		constant.ErrExpressionType,
		constant.ErrExpressionCostExceeded,
		constant.ErrExpressionEvaluation,
		constant.ErrExpressionProgram,
		constant.ErrExpressionCostEstimation,
		constant.ErrAmountExceedsPrecision,
		constant.ErrRuleNotFound,
		constant.ErrRuleNameAlreadyExists,
		constant.ErrRuleInvalidStatus,
		constant.ErrRuleEvaluationFailed,
		constant.ErrExpressionNotModifiable,
		constant.ErrRuleNilInput,
		constant.ErrRuleNameRequired,
		constant.ErrRuleNameTooLong,
		constant.ErrRuleExpressionRequired,
		constant.ErrRuleExpressionTooLong,
		constant.ErrRuleInvalidAction,
		constant.ErrRuleInvalidScope,
		constant.ErrRuleDescriptionTooLong,
		constant.ErrRuleScopesTooMany,
		constant.ErrRuleInvalidTransition,
		constant.ErrLimitNotFound,
		constant.ErrLimitInvalidStatusChange,
		constant.ErrLimitInvalidType,
		constant.ErrLimitInvalidMaxAmount,
		constant.ErrLimitInvalidCurrency,
		constant.ErrLimitInvalidScope,
		constant.ErrLimitNameRequired,
		constant.ErrLimitNameTooLong,
		constant.ErrLimitAlreadyDeleted,
		constant.ErrLimitNameInvalidChars,
		constant.ErrLimitDescriptionInvalidChars,
		constant.ErrLimitInvalidID,
		constant.ErrLimitDescriptionTooLong,
		constant.ErrLimitInvalidStatusFilter,
		constant.ErrLimitInvalidTypeFilter,
		constant.ErrLimitDeletedAtInvariant,
		constant.ErrLimitCheckFailed,
		constant.ErrLimitNilInput,
		constant.ErrLimitImmutableField,
		constant.ErrAuditEventNotFound,
		constant.ErrInvalidAuditEventFilters,
		constant.ErrAuditEventInvalidType,
		constant.ErrAuditEventInvalidAction,
		constant.ErrAuditEventInvalidResult,
		constant.ErrAuditEventResourceIDRequired,
		constant.ErrAuditEventInvalidResourceType,
		constant.ErrAuditEventActorIDRequired,
		constant.ErrAuditEventActorTypeInvalid,
		constant.ErrUsageCounterOverflow,
		constant.ErrUsageCounterLimitIDRequired,
		constant.ErrUsageCounterScopeKeyRequired,
		constant.ErrUsageCounterPeriodKeyRequired,
		constant.ErrUsageCounterCurrentUsageNegative,
		constant.ErrUsageCounterIncrementNonNegative,
		constant.ErrUsageCounterNotFound,
		constant.ErrUsageCounterExceedsLimit,
		constant.ErrUsageCounterDecrementNonNegative,
		constant.ErrCheckLimitsInvalidAmount,
		constant.ErrCheckLimitsInvalidCurrency,
		constant.ErrCheckLimitsUnknownLimitType,
		constant.ErrCheckLimitsInvalidTimestamp,
		constant.ErrCheckLimitsNilInput,
		constant.ErrCheckLimitsInvalidAccountID,
		constant.ErrCheckLimitsInvalidTransactionType,
		constant.ErrCheckLimitsInvalidSubType,
		constant.ErrCheckLimitsInvalidSegmentID,
		constant.ErrCheckLimitsInvalidPortfolioID,
		constant.ErrCheckLimitsInvalidMerchantID,
		constant.ErrLimitCheckerNilLimitRepo,
		constant.ErrLimitCheckerNilUsageCounterRepo,
		constant.ErrLimitCheckerNilClock,
		constant.ErrValidationRequestIDRequired,
		constant.ErrValidationInvalidTransactionType,
		constant.ErrValidationAmountNonPositive,
		constant.ErrValidationCurrencyRequired,
		constant.ErrValidationInvalidCurrency,
		constant.ErrValidationTimestampRequired,
		constant.ErrValidationTimestampFuture,
		constant.ErrValidationAccountRequired,
		constant.ErrValidationTimestampPast,
		constant.ErrValidationTimeout,
		constant.ErrValidationSegmentIDRequired,
		constant.ErrValidationPortfolioIDRequired,
		constant.ErrValidationSubTypeTooLong,
		constant.ErrValidationInvalidAccountType,
		constant.ErrValidationInvalidAccountStatus,
		constant.ErrValidationInvalidMerchantCategory,
		constant.ErrValidationInvalidMerchantCountry,
		constant.ErrValidationMerchantIDRequired,
		constant.ErrInvalidTransactionValidationFilters,
		constant.ErrTransactionValidationNotFound,
		constant.ErrListValidationsTimeout,
		constant.ErrTransactionValidationIDRequired,
		constant.ErrTransactionValidationCreatedAtRequired,
		constant.ErrRuleCacheWarmUpFailed,
		constant.ErrRuleCacheNotReady,
		constant.ErrLimitTimeWindowMismatch,
		constant.ErrLimitTimeWindowZeroWidth,
		constant.ErrTimeOfDayInvalidFormat,
		constant.ErrRuleNameAlreadyExistsInCtx,
		constant.ErrLimitNameAlreadyExists,
		constant.ErrLimitCustomDatesNotAllowed,
		constant.ErrLimitUnknownType,
		constant.ErrLimitCustomPeriodTooLong,
		constant.ErrLimitCustomPeriodExpired,
		constant.ErrLimitInvalidCustomStartFormat,
		constant.ErrLimitInvalidCustomEndFormat,
		constant.ErrLimitCustomDatesRequired,
		constant.ErrLimitCustomDatesOrder,
		constant.ErrMTConfigRequired,
		constant.ErrMTLoggerRequired,
		constant.ErrMTURLRequired,
		constant.ErrMTURLInvalid,
		constant.ErrMTServiceAPIKeyRequired,
		constant.ErrMTRedisHostRequired,
		constant.ErrMTPluginAuthRequired,
		constant.ErrMTAPIKeyOnlyValidationConfl,
		constant.ErrReadyzPgConnectionNotEstablished,
		constant.ErrReadyzPgConnectionFailed,
		constant.ErrReadyzPgPingFailed,
		constant.ErrReadyzDependenciesUnhealthy,
		constant.ErrReadyzCacheNotReady,
		constant.ErrReadyzCacheStale,
		constant.ErrSupervisorShuttingDown,
		constant.ErrTenantCapReached,
		constant.ErrSupervisorNilRuleCache,
		constant.ErrSupervisorNilSyncRepo,
		constant.ErrSupervisorNilUsageRepo,
		constant.ErrSupervisorNilCompiler,
		constant.ErrSupervisorNilLogger,
		constant.ErrSupervisorNilReaperRepo,
		constant.ErrSupervisorNilReaperAuditor,
		constant.ErrUnauthorizedMissingSub,
		constant.ErrReservationLimitIDRequired,
		constant.ErrReservationTransactionIDReq,
		constant.ErrReservationScopeKeyRequired,
		constant.ErrReservationPeriodKeyRequired,
		constant.ErrReservationAmountInvalid,
		constant.ErrReservationInvalidStatus,
		constant.ErrReservationExpiresAtRequired,
		constant.ErrReservationNotFound,
		constant.ErrReservationAlreadyTerminal,
		constant.ErrRouteNotFound,
		constant.ErrMethodNotAllowed,
		constant.ErrPendingTransactionLocked,
		constant.ErrReservationTenantRequired,
		constant.ErrInstrumentLedgerReferenceNotFound,
		constant.ErrInstrumentAccountReferenceNotFound,
		constant.ErrSkipNotPermitted,
		constant.ErrHolderRequired,
		constant.ErrHolderNotFound,
		constant.ErrInstrumentNotFound,
		constant.ErrDocumentAssociationError,
		constant.ErrAccountAlreadyAssociated,
		constant.ErrHolderHasInstruments,
		constant.ErrMetadataQueryInvalidFormat,
		constant.ErrMetadataQueryInvalidKey,
		constant.ErrMetadataQueryContainsOperator,
		constant.ErrInvalidHeaderValue,
		constant.ErrInstrumentClosingDateBeforeCreation,
		constant.ErrRelatedPartyNotFound,
		constant.ErrInvalidRelatedPartyRole,
		constant.ErrRelatedPartyDocumentRequired,
		constant.ErrRelatedPartyNameRequired,
		constant.ErrRelatedPartyStartDateRequired,
		constant.ErrRelatedPartyEndDateInvalid,
		constant.ErrHolderHasAccounts,
		constant.ErrKeysetNotFound,
		constant.ErrKeysetAlreadyExists,
		constant.ErrKeysetRevisionConflict,
		constant.ErrRegistryNotFound,
		constant.ErrRegistryAlreadyExists,
		constant.ErrRegistryRevisionConflict,
		constant.ErrOrganizationEncryptionFailed,
		constant.ErrProvisioningFailed,
		constant.ErrAuditEventRequired,
		constant.ErrAuditWriteFailed,
		constant.ErrReservedTenantID,
	}
}

// TestGolden_SentinelInventoryComplete guards allSentinels against drift: if a
// new constant.Err* is added to pkg/constant/errors.go without being added to the
// slice above, this count assertion fails and forces the sweep to be updated.
func TestGolden_SentinelInventoryComplete(t *testing.T) {
	t.Parallel()

	// pkg/constant/errors.go currently declares 423 Err* sentinels. Bump this
	// number in lockstep with allSentinels() whenever a code is added/removed.
	const wantSentinelCount = 423

	assert.Len(t, allSentinels(), wantSentinelCount,
		"sentinel inventory drifted: update allSentinels() and this count together")
}

// TestGolden_BusinessErrorCodeStatus is the full sweep (§5.2 source 1): every
// sentinel, driven through the REAL WithError dispatcher, must emit the (status,
// code) the frozen classifier derives. Mapped sentinels classify by their typed
// error; unmapped sentinels (the ~18 defined-but-unmapped) fall through
// ValidateBusinessError (returns the bare sentinel) and then through WithError to
// 500 / code 0046 — which is exactly what the classifier's fallthrough returns.
func TestGolden_BusinessErrorCodeStatus(t *testing.T) {
	t.Parallel()

	for _, sentinel := range allSentinels() {
		sentinel := sentinel
		validated := pkg.ValidateBusinessError(sentinel, "GoldenEntity")
		wantStatus, wantCode := classifyStatusOf(t, validated)

		t.Run(wantCode+"_"+strconv.Itoa(wantStatus)+"_"+sentinel.Error(), func(t *testing.T) {
			t.Parallel()

			gotStatus, gotCode := driveWithError(t, validated)
			assert.Equal(t, wantStatus, gotStatus, "MONEY-PATH: HTTP status for sentinel %q", sentinel.Error())
			assert.Equal(t, wantCode, gotCode, "MONEY-PATH: body[code] for sentinel %q", sentinel.Error())
		})
	}
}

// TestGolden_HelperPathCodeStatus covers §5.2 source 2: the sentinels reached not
// through ValidateBusinessError but through the three helper constructors, plus
// the named-case checks (FailedPreconditionError->500, fallthrough->500/0046).
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
			// FailedPreconditionError -> 500 (NOT 412) — §1 row 11 named case.
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
			// Fallthrough: an unclassifiable plain error -> WithError:110 ->
			// ValidateInternalError -> 500 / code 0046. Named case §5 point 5.
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
			// Int64 overflow -> 500. NOT 400.
			name:       "libcommons_overflow_int64_0097_500",
			err:        libCommons.Response{Code: libConstants.ErrOverFlowInt64.Error(), Message: "overflow"},
			wantStatus: fiber.StatusInternalServerError,
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
// quirk (§5.2 source 2, response.go:124): ValidateUnmarshallingError produces a
// ResponseError whose Code is "0094", and JSONResponseError writes the status as
// strconv.Atoi("0094") = 94. Go's net/http client (app.Test) rejects status 94 as
// malformed (< 100), so the raw fiber response is inspected directly instead of
// through an HTTP roundtrip. If the quirk ever changes (e.g. 0094 mapped to a real
// HTTP status), the raw status here changes and this test goes RED.
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
		DisableStartupMessage: true,
		ErrorHandler:          CanonicalFiberErrorHandler,
	})

	var writtenStatus int

	var writtenBody string

	app.Get("/probe", func(c *fiber.Ctx) error {
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

// TestGolden_ExplicitStatusArms covers §5.2 source 3 / §1.3: the two ambiguous
// codes whose status comes from renderCanonical (explicit status), NOT from the
// WithError code->status table. 0485 -> 405, 0143 -> 413. These must be pinned
// separately so the swap cannot collapse them into their table status (both 400).
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
				DisableStartupMessage: true,
				ErrorHandler:          CanonicalFiberErrorHandler,
			})
			app.Get("/probe", func(c *fiber.Ctx) error {
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
