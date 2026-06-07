# Error-Code Migration Table — Four-Family Consolidation

**Task 3.2.1** of the error consolidation (D1 outcome in `docs/standards/error-handling.md` E4: prefixed wire-code families `FEE-`, `TPL-`, `REP-`, `TRC-` are retired and folded into the single canonical numeric registry in `pkg/constant/errors.go`). This document is the authoring artifact: it maps every fork sentinel to a disposition (reuse an existing canonical code, or allocate a new sequential one), assigns the binding status class per E3/E4, and proposes the `constant.Entity*` names each family needs. **No code changes here.**

## Rules (restated)

1. **Reuse** a canonical code (0001–0178) only on exact semantic match — same meaning AND same E3 status class. Otherwise allocate a **new** code sequentially.
2. **Allocation blocks (contiguous, gap-free, per family):** fees `0179–0248` (70 codes), reporter `0249–0327` (79 codes), tracer `0328–0499` (172 codes). Next free code after all families: **`0500`**. Reused codes do NOT consume a slot in the family's block.
3. **Status class is binding (E3 + D2):** not-found→404, conflict/duplicate→409, malformed/syntactic input→400, business-rule/semantic→422, auth→401/403, infra→500/503. Where the assigned class differs from the fork's current typing, the row carries a `status-change` flag.
4. **Entity constants:** new `constant.Entity*` per family, following the style in `pkg/constant/entity.go` (PascalCase string value matching the Go domain noun).
5. **CRM-prefixed codes** in `pkg/constant/errors.go` (`CRM-00xx`) are already canonicalized post-shim and are **out of scope**; none collide with the new numeric blocks (they live in a separate `CRM-` string namespace).

## Typed-class legend

`ValidationError-400` · `Unprocessable-422` · `Conflict-409` · `NotFound-404` · `Internal-500` · `Unauthorized-401` · `Forbidden-403` · `FailedPrecondition-500` · `ServiceUnavailable-503`

The class is the platform typed-struct each canonical code maps to in `pkg/errors.go`'s `ValidateBusinessError` errorMap (the column states the target struct + HTTP status). Worker-internal `REP-`/some `TRC-` codes are carried by typed errors constructed at the failure site rather than routed through `ValidateBusinessError`; their target struct is given for the lock, with the relevant status.

---

## Family 1 — fees (`components/ledger/pkg/feeshared/constant/errors.go`)

70 `FEE-` sentinels. Block `0179–0248`. New codes assigned in declaration order, skipping reused canonical codes.

| old code | old sentinel | disposition | typed class | entity | status-change | note |
|---|---|---|---|---|---|---|
| FEE-0001 | ErrUnexpectedFieldsInTheRequest | reuse **0053** | ValidationError-400 | EntityPackage | — | exact match canonical ErrUnexpectedFieldsInTheRequest |
| FEE-0002 | ErrMissingFieldsInRequest | reuse **0009** | ValidationError-400 | EntityPackage | — | exact match canonical ErrMissingFieldsInRequest |
| FEE-0003 | ErrBadRequest | reuse **0047** | ValidationError-400 | EntityPackage | — | generic bad-request, canonical ErrBadRequest |
| FEE-0004 | ErrInternalServer | reuse **0046** | Internal-500 | EntityPackage | — | generic internal-server, canonical ErrInternalServer |
| FEE-0005 | ErrCalculationFieldType | new **0179** | ValidationError-400 | EntityFeeCalculation | — | invalid calculation field type |
| FEE-0006 | ErrInvalidQueryParameter | reuse **0082** | ValidationError-400 | EntityPackage | — | exact match canonical ErrInvalidQueryParameter |
| FEE-0007 | ErrInvalidDateFormat | reuse **0077** | ValidationError-400 | EntityPackage | — | exact match canonical ErrInvalidDateFormat |
| FEE-0008 | ErrInvalidFinalDate | reuse **0078** | ValidationError-400 | EntityPackage | — | exact match canonical ErrInvalidFinalDate |
| FEE-0009 | ErrDateRangeExceedsLimit | reuse **0079** | ValidationError-400 | EntityPackage | — | exact match canonical ErrDateRangeExceedsLimit |
| FEE-0010 | ErrInvalidDateRange | reuse **0083** | ValidationError-400 | EntityPackage | — | exact match canonical ErrInvalidDateRange |
| FEE-0011 | ErrPaginationLimitExceeded | reuse **0080** | ValidationError-400 | EntityPackage | — | exact match canonical ErrPaginationLimitExceeded |
| FEE-0012 | ErrEntityNotFound | reuse **0007** | NotFound-404 | EntityPackage | — | generic entity-not-found, canonical ErrEntityNotFound |
| FEE-0013 | ErrPriorityInvalid | new **0180** | ValidationError-400 | EntityPackage | — | priority value invalid |
| FEE-0014 | ErrFindAccountOnMidaz | new **0181** | Internal-500 | EntityFeeCalculation | — | upstream account lookup failure (infra) |
| FEE-0015 | ErrMinAmountGreaterThanMaxAmount | new **0182** | Unprocessable-422 | EntityPackage | 400→422 | semantic range rule (min>max) |
| FEE-0016 | ErrInvalidPathParameter | reuse **0065** | ValidationError-400 | EntityPackage | — | exact match canonical ErrInvalidPathParameter |
| FEE-0017 | ErrNothingToUpdate | new **0183** | ValidationError-400 | EntityPackage | — | empty PATCH body |
| FEE-0018 | ErrDuplicatePackage | new **0184** | Conflict-409 | EntityPackage | — | duplicate fee package |
| FEE-0019 | ErrInvalidHeaderParameter | new **0185** | ValidationError-400 | EntityPackage | — | malformed header (no canonical header-param code; CRM-0022 is CRM-namespaced) |
| FEE-0021 | ErrInvalidTransactionType | reuse **0072** | ValidationError-400 | EntityFeeCalculation | — | exact match canonical ErrInvalidTransactionType |
| FEE-0022 | ErrCalculateFee | new **0186** | Internal-500 | EntityFeeCalculation | — | fee calculation execution failure |
| FEE-0023 | ErrCalculationRequired | new **0187** | ValidationError-400 | EntityFeeCalculation | — | calculation block required |
| FEE-0024 | ErrPriorityOne | new **0188** | ValidationError-400 | EntityPackage | — | priority must start at one |
| FEE-0025 | ErrAppRuleFlatFeeAndPercentual | new **0189** | Unprocessable-422 | EntityPackage | 400→422 | mutually exclusive flat+percentual rule |
| FEE-0026 | ErrCalculationTypePercentual | new **0190** | ValidationError-400 | EntityFeeCalculation | — | percentual calc type constraint |
| FEE-0027 | ErrCalculationTypeFlatFee | new **0191** | ValidationError-400 | EntityFeeCalculation | — | flat-fee calc type constraint |
| FEE-0028 | ErrFeeFieldsRequired | new **0192** | ValidationError-400 | EntityPackage | — | required fee fields missing |
| FEE-0029 | ErrCalculationFieldOfFeeRequired | new **0193** | ValidationError-400 | EntityFeeCalculation | — | calculation field of fee required |
| FEE-0030 | ErrReferenceAmountInvalid | new **0194** | ValidationError-400 | EntityFeeCalculation | — | reference amount invalid |
| FEE-0031 | ErrAppRuleInvalid | new **0195** | ValidationError-400 | EntityPackage | — | application rule invalid |
| FEE-0032 | ErrCalculationTypeInvalid | new **0196** | ValidationError-400 | EntityFeeCalculation | — | calculation type invalid |
| FEE-0033 | ErrMaxAmountLessThanMinAmount | new **0197** | Unprocessable-422 | EntityPackage | 400→422 | semantic range rule (max<min) |
| FEE-0034 | ErrFilterPackage | new **0198** | ValidationError-400 | EntityPackage | — | package filter invalid |
| FEE-0035 | ErrPackageRange | new **0199** | Unprocessable-422 | EntityPackage | 400→422 | package amount-range overlap rule |
| FEE-0036 | ErrInvalidSortOrder | reuse **0081** | ValidationError-400 | EntityPackage | — | exact match canonical ErrInvalidSortOrder |
| FEE-0037 | ErrValidateDistributeTransactionValue | new **0200** | Unprocessable-422 | EntityFeeCalculation | 400→422 | distribute value does not reconcile (semantic) |
| FEE-0038 | ErrAppRuleMaxBetweenTypes | new **0201** | Unprocessable-422 | EntityPackage | 400→422 | max-between-types rule conflict |
| FEE-0039 | ErrInvalidSegmentID | new **0202** | ValidationError-400 | EntityPackage | — | segmentId malformed |
| FEE-0040 | ErrInvalidLedgerID | new **0203** | ValidationError-400 | EntityPackage | — | ledgerId malformed |
| FEE-0041 | ErrInvalidRequestBody | reuse **0094** | ValidationError-400 | EntityPackage | — | exact match canonical ErrInvalidRequestBody |
| FEE-0042 | ErrConvertToDecimal | new **0204** | Internal-500 | EntityFeeCalculation | — | decimal conversion failure |
| FEE-0043 | ErrIsDeductibleFrom | new **0205** | Unprocessable-422 | EntityFeeCalculation | 400→422 | deductible-from chain rule |
| FEE-0044 | ErrApplicationRule | new **0206** | ValidationError-400 | EntityPackage | — | application rule evaluation invalid |
| FEE-0045 | ErrForbiddenAccessMidaz | reuse **0043** | Forbidden-403 | EntityFeeCalculation | — | upstream authz denied, canonical ErrInsufficientPrivileges |
| FEE-0046 | ErrCalculationValuePercentage | new **0207** | ValidationError-400 | EntityFeeCalculation | — | percentage value invalid |
| FEE-0047 | ErrCalculationValueFlatFee | new **0208** | ValidationError-400 | EntityFeeCalculation | — | flat-fee value invalid |
| FEE-0048 | ErrAccessMidaz | new **0209** | Internal-500 | EntityFeeCalculation | — | upstream ledger call failed (availability/infra) |
| FEE-0049 | ErrDeductibleCalculationValuePercentage | new **0210** | ValidationError-400 | EntityFeeCalculation | — | deductible percentage value invalid |
| FEE-0050 | ErrDeductibleCalculationValueFlatFee | new **0211** | ValidationError-400 | EntityFeeCalculation | — | deductible flat-fee value invalid |
| FEE-0051 | ErrInvalidQueryParameterPage | new **0212** | ValidationError-400 | EntityPackage | — | page query param invalid (distinct from generic 0082: pagination-cursor specific) |
| FEE-0052 | ErrBillingPackageNotFound | new **0213** | NotFound-404 | EntityBillingPackage | — | billing package not found |
| FEE-0053 | ErrInvalidBillingPackageType | new **0214** | ValidationError-400 | EntityBillingPackage | — | invalid billing package type |
| FEE-0054 | ErrMissingVolumeFields | new **0215** | ValidationError-400 | EntityBillingPackage | — | volume pricing fields missing |
| FEE-0055 | ErrMissingMaintenanceFields | new **0216** | ValidationError-400 | EntityBillingPackage | — | maintenance pricing fields missing |
| FEE-0056 | ErrInvalidPricingModel | new **0217** | ValidationError-400 | EntityBillingPackage | — | invalid pricing model |
| FEE-0057 | ErrInvalidPricingTier | new **0218** | ValidationError-400 | EntityBillingPackage | — | invalid pricing tier |
| FEE-0058 | ErrBillingRouteOverlap | new **0219** | Unprocessable-422 | EntityBillingPackage | 400→422 | route overlap is a semantic conflict (not a 409 duplicate-id; overlapping ranges) |
| FEE-0059 | ErrTargetAccountNotFound | new **0220** | NotFound-404 | EntityBillingPackage | — | billing target account not found |
| FEE-0060 | ErrBillingCalculationFailed | new **0221** | Internal-500 | EntityBillingPackage | — | billing calculation execution failure |
| FEE-0061 | ErrNoActiveBillingPackages | new **0222** | NotFound-404 | EntityBillingPackage | — | no active billing packages (empty-result not-found) |
| FEE-0062 | ErrSegmentResolutionFailed | new **0223** | Internal-500 | EntityBillingPackage | — | segment resolution failure (upstream) |
| FEE-0063 | ErrInvalidBillingPeriod | new **0224** | ValidationError-400 | EntityBillingPackage | — | invalid billing period |
| FEE-0064 | ErrInvalidFreeQuota | new **0225** | ValidationError-400 | EntityBillingPackage | — | invalid free quota |
| FEE-0065 | ErrInvalidDiscountTier | new **0226** | ValidationError-400 | EntityBillingPackage | — | invalid discount tier |
| FEE-0067 | ErrInvalidCountMode | new **0227** | ValidationError-400 | EntityBillingPackage | — | invalid count mode |
| FEE-0068 | ErrMidazQueryFailed | new **0228** | Internal-500 | EntityBillingPackage | — | upstream midaz query failed (infra) |
| FEE-0069 | ErrInvalidAccountTarget | new **0229** | ValidationError-400 | EntityBillingPackage | — | invalid account target |
| FEE-0070 | ErrInvalidFeeAmount | new **0230** | ValidationError-400 | EntityFeeCalculation | — | invalid fee amount |
| FEE-0071 | ErrMissingSegmentContext | new **0231** | ValidationError-400 | EntityBillingPackage | — | segment context missing from request |
| FEE-0072 | ErrMidazRouteNotFound | new **0232** | NotFound-404 | EntityBillingPackage | — | upstream midaz route not found |

**fees usage of block:** reused 16, new 54 → highest new = **0232**. Block reserved `0179–0248` leaves `0233–0248` headroom for fee growth.

---

## Family 2 — reporter (`pkg/reporter/constant/errors.go`)

59 `TPL-` sentinels + 20 `REP-` const strings = **79** codes. Block `0249–0327`. TPL- first (declaration order), then REP-.

### TPL- sentinels

| old code | old sentinel | disposition | typed class | entity | status-change | note |
|---|---|---|---|---|---|---|
| TPL-0001 | ErrMissingRequiredFields | reuse **0009** | ValidationError-400 | EntityTemplate | — | == canonical ErrMissingFieldsInRequest |
| TPL-0002 | ErrInvalidFileFormat | new **0249** | ValidationError-400 | EntityTemplate | — | uploaded file format invalid |
| TPL-0003 | ErrInvalidOutputFormat | new **0250** | ValidationError-400 | EntityReport | — | invalid output format |
| TPL-0004 | ErrInvalidHeaderParameter | new **0251** | ValidationError-400 | EntityTemplate | — | malformed header param |
| TPL-0005 | ErrInvalidFileUploaded | new **0252** | ValidationError-400 | EntityTemplate | — | invalid uploaded file |
| TPL-0006 | ErrEmptyFile | new **0253** | ValidationError-400 | EntityTemplate | — | empty uploaded file (== fork ErrEmptyDSLFile semantics; canonical 0049 is DSL-specific, kept distinct) |
| TPL-0007 | ErrFileContentInvalid | new **0254** | ValidationError-400 | EntityTemplate | — | file content invalid |
| TPL-0008 | ErrInvalidMapFields | new **0255** | ValidationError-400 | EntityTemplate | — | invalid map fields |
| TPL-0009 | ErrInvalidPathParameter | reuse **0065** | ValidationError-400 | EntityTemplate | — | == canonical ErrInvalidPathParameter |
| TPL-0010 | ErrOutputFormatWithoutTemplateFile | new **0256** | ValidationError-400 | EntityReport | — | output format requires template file |
| TPL-0011 | ErrEntityNotFound | reuse **0007** | NotFound-404 | EntityTemplate | — | == canonical ErrEntityNotFound |
| TPL-0012 | ErrInvalidTemplateID | new **0257** | ValidationError-400 | EntityTemplate | — | template ID malformed |
| TPL-0013 | ErrInvalidLedgerIDList | new **0258** | ValidationError-400 | EntityTemplate | — | invalid ledger ID list |
| TPL-0014 | ErrMissingTableFields | new **0259** | ValidationError-400 | EntityTemplate | — | table fields missing |
| TPL-0015 | ErrUnexpectedFieldsInTheRequest | reuse **0053** | ValidationError-400 | EntityTemplate | — | == canonical ErrUnexpectedFieldsInTheRequest |
| TPL-0016 | ErrMissingFieldsInRequest | reuse **0009** | ValidationError-400 | EntityTemplate | — | == canonical ErrMissingFieldsInRequest |
| TPL-0017 | ErrBadRequest | reuse **0047** | ValidationError-400 | EntityTemplate | — | == canonical ErrBadRequest |
| TPL-0018 | ErrInternalServer | reuse **0046** | Internal-500 | EntityTemplate | — | == canonical ErrInternalServer |
| TPL-0019 | ErrInvalidQueryParameter | reuse **0082** | ValidationError-400 | EntityTemplate | — | == canonical ErrInvalidQueryParameter |
| TPL-0020 | ErrInvalidDateFormat | reuse **0077** | ValidationError-400 | EntityTemplate | — | == canonical ErrInvalidDateFormat |
| TPL-0021 | ErrInvalidFinalDate | reuse **0078** | ValidationError-400 | EntityTemplate | — | == canonical ErrInvalidFinalDate |
| TPL-0022 | ErrDateRangeExceedsLimit | reuse **0079** | ValidationError-400 | EntityTemplate | — | == canonical ErrDateRangeExceedsLimit |
| TPL-0023 | ErrInvalidDateRange | reuse **0083** | ValidationError-400 | EntityTemplate | — | == canonical ErrInvalidDateRange |
| TPL-0024 | ErrPaginationLimitExceeded | reuse **0080** | ValidationError-400 | EntityTemplate | — | == canonical ErrPaginationLimitExceeded |
| TPL-0025 | ErrInvalidSortOrder | reuse **0081** | ValidationError-400 | EntityTemplate | — | == canonical ErrInvalidSortOrder |
| TPL-0026 | ErrMetadataKeyLengthExceeded | reuse **0050** | ValidationError-400 | EntityTemplate | — | == canonical ErrMetadataKeyLengthExceeded |
| TPL-0027 | ErrMetadataValueLengthExceeded | reuse **0051** | ValidationError-400 | EntityTemplate | — | == canonical ErrMetadataValueLengthExceeded |
| TPL-0028 | ErrInvalidMetadataNesting | reuse **0067** | ValidationError-400 | EntityTemplate | — | == canonical ErrInvalidMetadataNesting |
| TPL-0029 | ErrReportStatusNotFinished | new **0260** | Unprocessable-422 | EntityReport | 400→422 | report not yet finished (state-precondition, semantic) |
| TPL-0030 | ErrMissingSchemaTable | new **0261** | ValidationError-400 | EntityDataSource | — | schema table missing |
| TPL-0031 | ErrMissingDataSource | new **0262** | ValidationError-400 | EntityDataSource | — | data source missing |
| TPL-0032 | ErrScriptTagDetected | new **0263** | ValidationError-400 | EntityTemplate | — | script tag detected (template security) |
| TPL-0033 | ErrDecryptionData | new **0264** | Internal-500 | EntityReport | — | data decryption failure (infra) |
| TPL-0034 | ErrCommunicateSeaweedFS | new **0265** | ServiceUnavailable-503 | EntityReport | — | object store unreachable (dependency availability) |
| TPL-0035 | ErrSchemaAmbiguous | new **0266** | Unprocessable-422 | EntityDataSource | 400→422 | schema resolves ambiguously (semantic) |
| TPL-0036 | ErrSchemaNotFound | new **0267** | NotFound-404 | EntityDataSource | — | schema not found |
| TPL-0037 | ErrTableNotFoundInSchema | new **0268** | NotFound-404 | EntityDataSource | — | table not found in schema |
| TPL-0038 | ErrDatabaseNotRegistered | new **0269** | FailedPrecondition-500 | EntityDataSource | — | target database not registered (config precondition) |
| TPL-0039 | ErrDuplicateRequestInFlight | new **0270** | Conflict-409 | EntityReport | — | duplicate in-flight request |
| TPL-0040 | ErrIdempotencyConflict | reuse **0084** | Conflict-409 | EntityReport | — | == canonical ErrIdempotencyKey (duplicate idempotency key) |
| TPL-0041 | ErrBucketRequired | new **0271** | ValidationError-400 | EntityReport | — | storage bucket required |
| TPL-0042 | ErrObjectKeyRequired | new **0272** | ValidationError-400 | EntityReport | — | object key required |
| TPL-0043 | ErrObjectNotFound | new **0273** | NotFound-404 | EntityReport | — | storage object not found |
| TPL-0044 | ErrTTLNotSupported | new **0274** | ValidationError-400 | EntityReport | — | TTL not supported by backend |
| TPL-0045 | ErrDuplicateDeadline | new **0275** | Conflict-409 | EntityDeadline | — | duplicate deadline |
| TPL-0046 | ErrInvalidDeadlineType | new **0276** | ValidationError-400 | EntityDeadline | — | invalid deadline type |
| TPL-0047 | ErrInvalidDeadlineFrequency | new **0277** | ValidationError-400 | EntityDeadline | — | invalid deadline frequency |
| TPL-0048 | ErrInvalidDeadlineColor | new **0278** | ValidationError-400 | EntityDeadline | — | invalid deadline color |
| TPL-0050 | ErrMonthsOfYearNotApplicable | new **0279** | ValidationError-400 | EntityDeadline | — | monthsOfYear not applicable for frequency |
| TPL-0052 | ErrMonthsOfYearRequired | new **0280** | ValidationError-400 | EntityDeadline | — | monthsOfYear required |
| TPL-0054 | ErrMonthsOfYearOutOfRange | new **0281** | ValidationError-400 | EntityDeadline | — | monthsOfYear out of range |
| TPL-0055 | ErrDueDateInPast | new **0282** | Unprocessable-422 | EntityDeadline | 400→422 | due date in past (semantic temporal rule) |
| TPL-0056 | ErrMonthsOfYearCountMismatch | new **0283** | ValidationError-400 | EntityDeadline | — | monthsOfYear count mismatch |
| TPL-0057 | ErrDataSourceNotFound | new **0284** | NotFound-404 | EntityDataSource | — | data source not found |
| TPL-0058 | ErrDataSourceUnavailable | new **0285** | ServiceUnavailable-503 | EntityDataSource | — | data source unavailable (dependency) |
| TPL-0059 | ErrSchemaValidationFailed | new **0286** | Unprocessable-422 | EntityDataSource | 400→422 | schema validation failed (semantic) |
| TPL-0060 | ErrExtractionJobFailed | new **0287** | Internal-500 | EntityReport | — | extraction job failure |
| TPL-0061 | ErrInvalidUTF8 | new **0288** | ValidationError-400 | EntityReport | — | invalid UTF-8 in extracted data |
| TPL-0062 | ErrTemplateRenderFailed | new **0289** | Internal-500 | EntityTemplate | — | template render failure |

### REP- const strings (worker-internal pipeline codes)

These are carried by typed errors (`ValidationError`, `FailedPreconditionError`) constructed at the failure site, NOT routed through `ValidateBusinessError`. They surface in worker failure metadata (E9: classified `error_code`, never raw text). Each migrates to a numeric code; the target struct is given for the per-surface lock (E14).

| old code | old sentinel | disposition | typed class | entity | status-change | note |
|---|---|---|---|---|---|---|
| REP-0060 | ErrCodeDataSourceNotFound | new **0290** | NotFound-404 | EntityDataSource | — | data source not found (worker path) |
| REP-0061 | ErrCodeDataSourceUnavailable | new **0291** | ServiceUnavailable-503 | EntityDataSource | — | data source unavailable (worker path) |
| REP-0062 | ErrCodeUnsupportedDatabaseType | new **0292** | FailedPrecondition-500 | EntityDataSource | — | unsupported database type |
| REP-0063 | ErrCodeUnexpectedSchemaResult | new **0293** | Internal-500 | EntityDataSource | — | unexpected schema result |
| REP-0064 | ErrCodeUnexpectedTableResult | new **0294** | Internal-500 | EntityDataSource | — | unexpected table result |
| REP-0065 | ErrCodeUnexpectedCollectionResult | new **0295** | Internal-500 | EntityDataSource | — | unexpected collection result |
| REP-0066 | ErrCodeCRMHashKeyNotConfigured | new **0296** | FailedPrecondition-500 | EntityReport | — | CRM hash key not configured |
| REP-0067 | ErrCodeCRMEncryptKeyNotConfigured | new **0297** | FailedPrecondition-500 | EntityReport | — | CRM encrypt key not configured |
| REP-0068 | ErrCodeCipherInitFailed | new **0298** | Internal-500 | EntityReport | — | cipher init failed |
| REP-0069 | ErrCodeRecordDecryptionFailed | new **0299** | Internal-500 | EntityReport | — | record decryption failed |
| REP-0070 | ErrCodeStorageNotConfigured | new **0300** | FailedPrecondition-500 | EntityReport | — | storage not configured |
| REP-0072 | ErrCodeInvalidExtractedData | new **0301** | Internal-500 | EntityReport | — | invalid extracted data |
| REP-0073 | ErrCodeEmptyEncryptedData | new **0302** | Internal-500 | EntityReport | — | empty encrypted data |
| REP-0074 | ErrCodeDecryptionKeyNotConfigured | new **0303** | FailedPrecondition-500 | EntityReport | — | decryption key not configured |
| REP-0075 | ErrCodeInvalidEncryptedData | new **0304** | Internal-500 | EntityReport | — | invalid encrypted data |
| REP-0076 | ErrCodeAESCipherCreationFailed | new **0305** | Internal-500 | EntityReport | — | AES cipher creation failed |
| REP-0077 | ErrCodeGCMCreationFailed | new **0306** | Internal-500 | EntityReport | — | GCM creation failed |
| REP-0078 | ErrCodeCorruptEncryptedData | new **0307** | Internal-500 | EntityReport | — | corrupt encrypted data |
| REP-0079 | ErrCodeAESGCMDecryptionFailed | new **0308** | Internal-500 | EntityReport | — | AES-GCM decryption failed |
| REP-0080 | ErrCodeInvalidFetcherResponse | new **0309** | Internal-500 | EntityDataSource | — | invalid fetcher response |

**reporter usage of block:** reused 18 TPL codes, new 41 TPL + 20 REP = 61 new → highest new = **0309**. Block reserved `0249–0327` leaves `0310–0327` headroom. (REP-0071 and REP-0081 are already-retired gaps in the fork; not migrated.)

---

## Family 3 — tracer (`components/tracer/pkg/constant/errors.go`)

172 `TRC-` sentinels + 10 `Code*` HTTP const aliases. Block `0328–0499`. The 10 `Code*` consts alias existing TRC sentinels (e.g. `CodeBadRequest = "TRC-0003"`) → they map to the **same** new code as the aliased sentinel and consume **no** extra slot; they are listed in the alias sub-table at the end. New codes assigned in declaration order, skipping reuses.

| old code | old sentinel | disposition | typed class | entity | status-change | note |
|---|---|---|---|---|---|---|
| TRC-0001 | ErrUnexpectedFieldsInTheRequest | reuse **0053** | ValidationError-400 | EntityRule | — | == canonical ErrUnexpectedFieldsInTheRequest |
| TRC-0002 | ErrMissingFieldsInRequest | reuse **0009** | ValidationError-400 | EntityRule | — | == canonical ErrMissingFieldsInRequest |
| TRC-0003 | ErrBadRequest | reuse **0047** | ValidationError-400 | EntityRule | — | == canonical ErrBadRequest |
| TRC-0004 | ErrInternalServer | reuse **0046** | Internal-500 | EntityRule | — | == canonical ErrInternalServer |
| TRC-0005 | ErrCalculationFieldType | new **0328** | ValidationError-400 | EntityRule | — | invalid calculation field type |
| TRC-0006 | ErrInvalidQueryParameter | reuse **0082** | ValidationError-400 | EntityRule | — | == canonical ErrInvalidQueryParameter |
| TRC-0007 | ErrInvalidPathParameter | reuse **0065** | ValidationError-400 | EntityRule | — | == canonical ErrInvalidPathParameter |
| TRC-0010 | ErrParentIDNotFound | new **0329** | NotFound-404 | EntityRule | — | parent ID not found |
| TRC-0011 | ErrPayloadTooLarge | reuse **0143** | ValidationError-400 | EntityRule | — | == canonical ErrPayloadTooLarge |
| TRC-0012 | ErrContextCancelled | new **0330** | ServiceUnavailable-503 | EntityRule | — | context cancelled / service unavailable |
| TRC-0020 | ErrInvalidDateFormat | reuse **0077** | ValidationError-400 | EntityRule | — | == canonical ErrInvalidDateFormat |
| TRC-0021 | ErrInvalidFinalDate | reuse **0078** | ValidationError-400 | EntityRule | — | == canonical ErrInvalidFinalDate |
| TRC-0022 | ErrDateRangeExceedsLimit | reuse **0079** | ValidationError-400 | EntityRule | — | == canonical ErrDateRangeExceedsLimit |
| TRC-0023 | ErrInvalidDateRange | reuse **0083** | ValidationError-400 | EntityRule | — | == canonical ErrInvalidDateRange |
| TRC-0040 | ErrPaginationLimitExceeded | reuse **0080** | ValidationError-400 | EntityRule | — | == canonical ErrPaginationLimitExceeded |
| TRC-0041 | ErrPaginationLimitInvalid | new **0331** | ValidationError-400 | EntityRule | — | pagination limit must be positive |
| TRC-0042 | ErrInvalidSortOrder | reuse **0081** | ValidationError-400 | EntityRule | — | == canonical ErrInvalidSortOrder |
| TRC-0043 | ErrInvalidSortColumn | new **0332** | ValidationError-400 | EntityRule | — | sort column not in allowlist |
| TRC-0044 | ErrInvalidCursor | new **0333** | ValidationError-400 | EntityRule | — | invalid/corrupted pagination cursor |
| TRC-0045 | ErrCursorWithSortParams | new **0334** | ValidationError-400 | EntityRule | — | cursor + sort mutually exclusive |
| TRC-0060 | ErrMetadataKeyLengthExceeded | reuse **0050** | ValidationError-400 | EntityRule | — | == canonical ErrMetadataKeyLengthExceeded |
| TRC-0061 | ErrMetadataValueLengthExceeded | reuse **0051** | ValidationError-400 | EntityRule | — | == canonical ErrMetadataValueLengthExceeded |
| TRC-0062 | ErrInvalidMetadataNesting | reuse **0067** | ValidationError-400 | EntityRule | — | == canonical ErrInvalidMetadataNesting |
| TRC-0063 | ErrMetadataEntriesExceeded | new **0335** | ValidationError-400 | EntityRule | — | metadata entries exceed max 50 |
| TRC-0064 | ErrMetadataKeyInvalidChars | new **0336** | ValidationError-400 | EntityRule | — | metadata key invalid chars |
| TRC-0080 | ErrInvalidDecision | new **0337** | ValidationError-400 | EntityRule | — | invalid decision value |
| TRC-0081 | ErrReasonRequired | new **0338** | ValidationError-400 | EntityRule | — | reason required |
| TRC-0082 | ErrInvalidDefaultDecision | new **0339** | ValidationError-400 | EntityRule | — | invalid default decision |
| TRC-0083 | ErrExpressionSyntax | new **0340** | ValidationError-400 | EntityRule | — | invalid CEL syntax |
| TRC-0084 | ErrExpressionType | new **0341** | ValidationError-400 | EntityRule | — | expression must return boolean |
| TRC-0085 | ErrExpressionCostExceeded | new **0342** | Unprocessable-422 | EntityRule | 400→422 | CEL cost over threshold (semantic resource rule) |
| TRC-0086 | ErrExpressionEvaluation | new **0343** | Internal-500 | EntityRule | — | runtime CEL evaluation error |
| TRC-0087 | ErrExpressionProgram | new **0344** | ValidationError-400 | EntityRule | — | program creation failed (compile phase) |
| TRC-0088 | ErrExpressionCostEstimation | new **0345** | Internal-500 | EntityRule | — | failed to estimate expression cost |
| TRC-0089 | ErrAmountExceedsPrecision | new **0346** | Unprocessable-422 | EntityValidationRequest | 400→422 | amount exceeds CEL float64 safe precision (semantic) |
| TRC-0100 | ErrRuleNotFound | new **0347** | NotFound-404 | EntityRule | — | rule not found by ID |
| TRC-0101 | ErrRuleNameAlreadyExists | new **0348** | Conflict-409 | EntityRule | — | rule name must be unique |
| TRC-0102 | ErrRuleInvalidStatus | new **0349** | Unprocessable-422 | EntityRule | 400→422 | invalid rule status transition (semantic) |
| TRC-0103 | ErrRuleEvaluationFailed | new **0350** | Internal-500 | EntityRule | — | rule evaluation failed |
| TRC-0104 | ErrExpressionNotModifiable | new **0351** | Unprocessable-422 | EntityRule | 400→422 | expression immutable for non-DRAFT (state rule) |
| TRC-0105 | ErrRuleNilInput | new **0352** | ValidationError-400 | EntityRule | — | rule input cannot be nil |
| TRC-0106 | ErrRuleNameRequired | new **0353** | ValidationError-400 | EntityRule | — | rule name required |
| TRC-0107 | ErrRuleNameTooLong | new **0354** | ValidationError-400 | EntityRule | — | rule name exceeds 255 |
| TRC-0108 | ErrRuleExpressionRequired | new **0355** | ValidationError-400 | EntityRule | — | rule expression required |
| TRC-0109 | ErrRuleExpressionTooLong | new **0356** | ValidationError-400 | EntityRule | — | rule expression exceeds 5000 |
| TRC-0110 | ErrRuleInvalidAction | new **0357** | ValidationError-400 | EntityRule | — | action must be ALLOW/DENY/REVIEW |
| TRC-0111 | ErrRuleInvalidScope | new **0358** | ValidationError-400 | EntityRule | — | scope must have ≥1 field |
| TRC-0112 | ErrRuleDescriptionTooLong | new **0359** | ValidationError-400 | EntityRule | — | rule description exceeds 1000 |
| TRC-0113 | ErrRuleScopesTooMany | new **0360** | ValidationError-400 | EntityRule | — | rule scopes exceed 100 |
| TRC-0114 | ErrRuleInvalidTransition | new **0361** | Unprocessable-422 | EntityRule | 400→422 | status transition not allowed (semantic) |
| TRC-0120 | ErrLimitNotFound | new **0362** | NotFound-404 | EntityLimit | — | limit not found by ID |
| TRC-0121 | ErrLimitInvalidStatusChange | new **0363** | Unprocessable-422 | EntityLimit | 400→422 | invalid limit status transition (semantic) |
| TRC-0122 | ErrLimitInvalidType | new **0364** | ValidationError-400 | EntityLimit | — | invalid limit type |
| TRC-0123 | ErrLimitInvalidMaxAmount | new **0365** | ValidationError-400 | EntityLimit | — | maxAmount must be positive |
| TRC-0124 | ErrLimitInvalidCurrency | new **0366** | ValidationError-400 | EntityLimit | — | currency must be ISO 4217 |
| TRC-0125 | ErrLimitInvalidScope | new **0367** | ValidationError-400 | EntityLimit | — | scope validation failed |
| TRC-0126 | ErrLimitNameRequired | new **0368** | ValidationError-400 | EntityLimit | — | limit name required |
| TRC-0127 | ErrLimitNameTooLong | new **0369** | ValidationError-400 | EntityLimit | — | limit name too long |
| TRC-0128 | ErrLimitAlreadyDeleted | new **0370** | Unprocessable-422 | EntityLimit | 400→422 | already DELETED (state rule) |
| TRC-0129 | ErrLimitNameInvalidChars | new **0371** | ValidationError-400 | EntityLimit | — | limit name invalid chars |
| TRC-0130 | ErrLimitDescriptionInvalidChars | new **0372** | ValidationError-400 | EntityLimit | — | limit description invalid chars |
| TRC-0131 | ErrLimitInvalidID | new **0373** | ValidationError-400 | EntityLimit | — | limit ID invalid/nil |
| TRC-0132 | ErrLimitDescriptionTooLong | new **0374** | ValidationError-400 | EntityLimit | — | limit description too long |
| TRC-0133 | ErrLimitInvalidStatusFilter | new **0375** | ValidationError-400 | EntityLimit | — | invalid status filter |
| TRC-0134 | ErrLimitInvalidTypeFilter | new **0376** | ValidationError-400 | EntityLimit | — | invalid limitType filter |
| TRC-0135 | ErrLimitDeletedAtInvariant | new **0377** | Internal-500 | EntityLimit | — | DeletedAt-iff-DELETED invariant breach (data integrity) |
| TRC-0136 | ErrLimitCheckFailed | new **0378** | Internal-500 | EntityLimit | — | limit check failed |
| TRC-0137 | ErrLimitNilInput | new **0379** | ValidationError-400 | EntityLimit | — | limit input cannot be nil |
| TRC-0138 | ErrLimitImmutableField | new **0380** | Unprocessable-422 | EntityLimit | 400→422 | immutable field (limitType, currency) — state rule |
| TRC-0140 | ErrAuditEventNotFound | new **0381** | NotFound-404 | EntityAuditEvent | — | audit event not found |
| TRC-0141 | ErrInvalidAuditEventFilters | new **0382** | ValidationError-400 | EntityAuditEvent | — | invalid audit event filters |
| TRC-0142 | ErrAuditEventInvalidType | new **0383** | ValidationError-400 | EntityAuditEvent | — | invalid audit event type |
| TRC-0143 | ErrAuditEventInvalidAction | new **0384** | ValidationError-400 | EntityAuditEvent | — | invalid audit action |
| TRC-0144 | ErrAuditEventInvalidResult | new **0385** | ValidationError-400 | EntityAuditEvent | — | invalid audit result |
| TRC-0145 | ErrAuditEventResourceIDRequired | new **0386** | ValidationError-400 | EntityAuditEvent | — | resource ID required |
| TRC-0146 | ErrAuditEventInvalidResourceType | new **0387** | ValidationError-400 | EntityAuditEvent | — | invalid resource type |
| TRC-0147 | ErrAuditEventActorIDRequired | new **0388** | ValidationError-400 | EntityAuditEvent | — | actor ID required |
| TRC-0148 | ErrAuditEventActorTypeInvalid | new **0389** | ValidationError-400 | EntityAuditEvent | — | actor type must be user/system |
| TRC-0160 | ErrUsageCounterOverflow | new **0390** | Internal-500 | EntityUsageCounter | — | usage counter would overflow (arithmetic invariant) |
| TRC-0161 | ErrUsageCounterLimitIDRequired | new **0391** | ValidationError-400 | EntityUsageCounter | — | limitID required |
| TRC-0162 | ErrUsageCounterScopeKeyRequired | new **0392** | ValidationError-400 | EntityUsageCounter | — | scopeKey required |
| TRC-0163 | ErrUsageCounterPeriodKeyRequired | new **0393** | ValidationError-400 | EntityUsageCounter | — | periodKey required |
| TRC-0164 | ErrUsageCounterCurrentUsageNegative | new **0394** | ValidationError-400 | EntityUsageCounter | — | currentUsage must be non-negative |
| TRC-0165 | ErrUsageCounterIncrementNonNegative | new **0395** | ValidationError-400 | EntityUsageCounter | — | increment must be non-negative |
| TRC-0166 | ErrUsageCounterNotFound | new **0396** | NotFound-404 | EntityUsageCounter | — | usage counter not found |
| TRC-0167 | ErrUsageCounterExceedsLimit | new **0397** | Unprocessable-422 | EntityUsageCounter | 400→422 | increment would exceed limit max (semantic) |
| TRC-0168 | ErrUsageCounterDecrementNonNegative | new **0398** | ValidationError-400 | EntityUsageCounter | — | decrement must be non-negative |
| TRC-0180 | ErrCheckLimitsInvalidAmount | new **0399** | ValidationError-400 | EntityValidationRequest | — | check-limits amount must be positive |
| TRC-0181 | ErrCheckLimitsInvalidCurrency | new **0400** | ValidationError-400 | EntityValidationRequest | — | check-limits currency must be ISO 4217 |
| TRC-0183 | ErrCheckLimitsUnknownLimitType | new **0401** | ValidationError-400 | EntityValidationRequest | — | unknown limit type for period key |
| TRC-0184 | ErrCheckLimitsInvalidTimestamp | new **0402** | ValidationError-400 | EntityValidationRequest | — | timestamp must not be zero |
| TRC-0185 | ErrCheckLimitsNilInput | new **0403** | ValidationError-400 | EntityValidationRequest | — | check-limits input cannot be nil |
| TRC-0186 | ErrCheckLimitsInvalidAccountID | new **0404** | ValidationError-400 | EntityValidationRequest | — | accountId required |
| TRC-0187 | ErrCheckLimitsInvalidTransactionType | new **0405** | ValidationError-400 | EntityValidationRequest | — | transactionType must be valid |
| TRC-0188 | ErrCheckLimitsInvalidSubType | new **0406** | ValidationError-400 | EntityValidationRequest | — | subType exceeds max length |
| TRC-0189 | ErrCheckLimitsInvalidSegmentID | new **0407** | ValidationError-400 | EntityValidationRequest | — | segmentId must not be zero UUID |
| TRC-0190 | ErrCheckLimitsInvalidPortfolioID | new **0408** | ValidationError-400 | EntityValidationRequest | — | portfolioId must not be zero UUID |
| TRC-0191 | ErrCheckLimitsInvalidMerchantID | new **0409** | ValidationError-400 | EntityValidationRequest | — | merchantId must not be zero UUID |
| TRC-0200 | ErrLimitCheckerNilLimitRepo | new **0410** | Internal-500 | EntityLimit | — | DI: limit repo nil (constructor invariant) |
| TRC-0201 | ErrLimitCheckerNilUsageCounterRepo | new **0411** | Internal-500 | EntityLimit | — | DI: usage counter repo nil |
| TRC-0202 | ErrLimitCheckerNilClock | new **0412** | Internal-500 | EntityLimit | — | DI: clock nil |
| TRC-0220 | ErrValidationRequestIDRequired | new **0413** | ValidationError-400 | EntityValidationRequest | — | requestId required |
| TRC-0221 | ErrValidationInvalidTransactionType | new **0414** | ValidationError-400 | EntityValidationRequest | — | invalid transactionType |
| TRC-0222 | ErrValidationAmountNonPositive | new **0415** | ValidationError-400 | EntityValidationRequest | — | amount must be positive |
| TRC-0223 | ErrValidationCurrencyRequired | new **0416** | ValidationError-400 | EntityValidationRequest | — | currency required |
| TRC-0224 | ErrValidationInvalidCurrency | new **0417** | ValidationError-400 | EntityValidationRequest | — | currency must be ISO 4217 |
| TRC-0225 | ErrValidationTimestampRequired | new **0418** | ValidationError-400 | EntityValidationRequest | — | timestamp required |
| TRC-0226 | ErrValidationTimestampFuture | new **0419** | ValidationError-400 | EntityValidationRequest | — | timestamp cannot be future |
| TRC-0227 | ErrValidationAccountRequired | new **0420** | ValidationError-400 | EntityValidationRequest | — | account required |
| TRC-0228 | ErrValidationTimestampPast | new **0421** | ValidationError-400 | EntityValidationRequest | — | timestamp too far in past |
| TRC-0229 | ErrValidationTimeout | new **0422** | ServiceUnavailable-503 | EntityValidationRequest | — | validation timeout (deadline exceeded) |
| TRC-0230 | ErrValidationSegmentIDRequired | new **0423** | ValidationError-400 | EntityValidationRequest | — | segmentId required when segment provided |
| TRC-0231 | ErrValidationPortfolioIDRequired | new **0424** | ValidationError-400 | EntityValidationRequest | — | portfolioId required when portfolio provided |
| TRC-0232 | ErrValidationSubTypeTooLong | new **0425** | ValidationError-400 | EntityValidationRequest | — | subType exceeds max 50 |
| TRC-0233 | ErrValidationInvalidAccountType | new **0426** | ValidationError-400 | EntityValidationRequest | — | account.type must be checking/savings/credit |
| TRC-0234 | ErrValidationInvalidAccountStatus | new **0427** | ValidationError-400 | EntityValidationRequest | — | account.status must be active/suspended/closed |
| TRC-0235 | ErrValidationInvalidMerchantCategory | new **0428** | ValidationError-400 | EntityValidationRequest | — | merchant.category must be 4-digit MCC |
| TRC-0236 | ErrValidationInvalidMerchantCountry | new **0429** | ValidationError-400 | EntityValidationRequest | — | merchant.country must be ISO 3166-1 alpha-2 |
| TRC-0237 | ErrValidationMerchantIDRequired | new **0430** | ValidationError-400 | EntityValidationRequest | — | merchant.id required when merchant provided |
| TRC-0250 | ErrInvalidTransactionValidationFilters | new **0431** | ValidationError-400 | EntityTransactionValidation | — | invalid filter params |
| TRC-0251 | ErrTransactionValidationNotFound | new **0432** | NotFound-404 | EntityTransactionValidation | — | validation record not found |
| TRC-0252 | ErrListValidationsTimeout | new **0433** | ServiceUnavailable-503 | EntityTransactionValidation | — | list query timeout (deadline exceeded) |
| TRC-0270 | ErrTransactionValidationIDRequired | new **0434** | ValidationError-400 | EntityTransactionValidation | — | validation ID required |
| TRC-0271 | ErrTransactionValidationCreatedAtRequired | new **0435** | ValidationError-400 | EntityTransactionValidation | — | createdAt required |
| TRC-0280 | ErrRuleCacheWarmUpFailed | new **0436** | FailedPrecondition-500 | EntityRule | — | rule cache warm-up failed (boot precondition) |
| TRC-0281 | ErrRuleCacheNotReady | new **0437** | ServiceUnavailable-503 | EntityRule | — | rule cache not ready (readiness) |
| TRC-0300 | ErrLimitTimeWindowMismatch | new **0438** | ValidationError-400 | EntityLimit | — | activeTimeStart/End must both be set or nil |
| TRC-0301 | ErrLimitTimeWindowZeroWidth | new **0439** | ValidationError-400 | EntityLimit | — | activeTimeStart cannot equal End |
| TRC-0302 | ErrTimeOfDayInvalidFormat | new **0440** | ValidationError-400 | EntityLimit | — | time-of-day format must be HH:MM |
| TRC-0303 | ErrRuleNameAlreadyExistsInCtx | new **0441** | Conflict-409 | EntityRule | — | rule name already exists in context |
| TRC-0304 | ErrLimitNameAlreadyExists | new **0442** | Conflict-409 | EntityLimit | — | limit name already exists |
| TRC-0305 | ErrLimitCustomDatesNotAllowed | new **0443** | ValidationError-400 | EntityLimit | — | custom dates only for CUSTOM type |
| TRC-0306 | ErrLimitUnknownType | new **0444** | ValidationError-400 | EntityLimit | — | unknown limit type |
| TRC-0307 | ErrLimitCustomPeriodTooLong | new **0445** | Unprocessable-422 | EntityLimit | 400→422 | custom period > 5 years (semantic) |
| TRC-0308 | ErrLimitCustomPeriodExpired | new **0446** | Unprocessable-422 | EntityLimit | 400→422 | custom period end must be future (semantic temporal) |
| TRC-0309 | ErrLimitInvalidCustomStartFormat | new **0447** | ValidationError-400 | EntityLimit | — | customStartDate format must be RFC3339 |
| TRC-0310 | ErrLimitInvalidCustomEndFormat | new **0448** | ValidationError-400 | EntityLimit | — | customEndDate format must be RFC3339 |
| TRC-0311 | ErrLimitCustomDatesRequired | new **0449** | ValidationError-400 | EntityLimit | — | custom dates required for CUSTOM type |
| TRC-0312 | ErrLimitCustomDatesOrder | new **0450** | ValidationError-400 | EntityLimit | — | customStartDate must precede customEndDate |
| TRC-0320 | ErrMTConfigRequired | new **0451** | Internal-500 | EntityRule | — | MT bootstrap: cfg required (config precondition) |
| TRC-0321 | ErrMTLoggerRequired | new **0452** | Internal-500 | EntityRule | — | MT bootstrap: logger required |
| TRC-0322 | ErrMTURLRequired | new **0453** | FailedPrecondition-500 | EntityRule | — | MULTI_TENANT_URL required |
| TRC-0323 | ErrMTURLInvalid | new **0454** | FailedPrecondition-500 | EntityRule | — | MULTI_TENANT_URL must be valid absolute URL |
| TRC-0324 | ErrMTServiceAPIKeyRequired | new **0455** | FailedPrecondition-500 | EntityRule | — | MULTI_TENANT_SERVICE_API_KEY required |
| TRC-0325 | ErrMTRedisHostRequired | new **0456** | FailedPrecondition-500 | EntityRule | — | MULTI_TENANT_REDIS_HOST required |
| TRC-0326 | ErrMTPluginAuthRequired | new **0457** | FailedPrecondition-500 | EntityRule | — | MT requires PLUGIN_AUTH_ENABLED=true |
| TRC-0327 | ErrMTAPIKeyOnlyValidationConfl | new **0458** | FailedPrecondition-500 | EntityRule | — | MT incompatible with API_KEY_ENABLED_ONLY_VALIDATION |
| TRC-0328 | ErrReadyzPgConnectionNotEstablished | new **0459** | ServiceUnavailable-503 | EntityRule | — | postgres readyz: connection not established |
| TRC-0329 | ErrReadyzPgConnectionFailed | new **0460** | ServiceUnavailable-503 | EntityRule | — | postgres readyz: connection failed |
| TRC-0330 | ErrReadyzPgPingFailed | new **0461** | ServiceUnavailable-503 | EntityRule | — | postgres readyz: ping failed |
| TRC-0331 | ErrReadyzDependenciesUnhealthy | new **0462** | ServiceUnavailable-503 | EntityRule | — | /readyz aggregate: dependencies unhealthy |
| TRC-0332 | ErrReadyzCacheNotReady | new **0463** | ServiceUnavailable-503 | EntityRule | — | rule_cache readyz: not ready |
| TRC-0333 | ErrReadyzCacheStale | new **0464** | ServiceUnavailable-503 | EntityRule | — | rule_cache readyz: data stale |
| TRC-0334 | ErrSupervisorShuttingDown | new **0465** | ServiceUnavailable-503 | EntityRule | — | worker supervisor shutting down |
| TRC-0335 | ErrTenantCapReached | new **0466** | ServiceUnavailable-503 | EntityRule | — | tenant worker cap reached; retry after backoff |
| TRC-0336 | ErrSupervisorNilRuleCache | new **0467** | Internal-500 | EntityRule | — | supervisor DI: rule cache required |
| TRC-0337 | ErrSupervisorNilSyncRepo | new **0468** | Internal-500 | EntityRule | — | supervisor DI: sync repo required |
| TRC-0338 | ErrSupervisorNilUsageRepo | new **0469** | Internal-500 | EntityUsageCounter | — | supervisor DI: usage repo required |
| TRC-0339 | ErrSupervisorNilCompiler | new **0470** | Internal-500 | EntityRule | — | supervisor DI: compiler required |
| TRC-0340 | ErrSupervisorNilLogger | new **0471** | Internal-500 | EntityRule | — | supervisor DI: logger required |
| TRC-0341 | ErrSupervisorNilReaperRepo | new **0472** | Internal-500 | EntityReservation | — | supervisor DI: reaper repo required |
| TRC-0342 | ErrSupervisorNilReaperAuditor | new **0473** | Internal-500 | EntityReservation | — | supervisor DI: reaper auditor required |
| TRC-0350 | ErrUnauthorizedMissingSub | new **0474** | Unauthorized-401 | EntityRule | — | JWT lacks 'sub' claim |
| TRC-0370 | ErrReservationLimitIDRequired | new **0475** | ValidationError-400 | EntityReservation | — | reservation: limitId required |
| TRC-0371 | ErrReservationTransactionIDReq | new **0476** | ValidationError-400 | EntityReservation | — | reservation: transactionId required |
| TRC-0372 | ErrReservationScopeKeyRequired | new **0477** | ValidationError-400 | EntityReservation | — | reservation: scopeKey required |
| TRC-0373 | ErrReservationPeriodKeyRequired | new **0478** | ValidationError-400 | EntityReservation | — | reservation: periodKey required |
| TRC-0374 | ErrReservationAmountInvalid | new **0479** | ValidationError-400 | EntityReservation | — | reservation: amount must be non-negative |
| TRC-0375 | ErrReservationInvalidStatus | new **0480** | ValidationError-400 | EntityReservation | — | reservation: invalid status value |
| TRC-0376 | ErrReservationExpiresAtRequired | new **0481** | ValidationError-400 | EntityReservation | — | reservation: reservationExpiresAt required |
| TRC-0377 | ErrReservationNotFound | new **0482** | NotFound-404 | EntityReservation | — | reservation not found |
| TRC-0378 | ErrReservationAlreadyTerminal | new **0483** | Unprocessable-422 | EntityReservation | 400→422 | reservation already terminal (state rule) |

### tracer `Code*` HTTP-const aliases (no new slot)

Each aliases an existing TRC sentinel → maps to the same new code as that sentinel.

| HTTP const | aliases sentinel | → new code |
|---|---|---|
| CodeBadRequest = "TRC-0003" | ErrBadRequest | reuse **0047** |
| CodeInternalServer = "TRC-0004" | ErrInternalServer | reuse **0046** |
| CodePayloadTooLarge = "TRC-0011" | ErrPayloadTooLarge | reuse **0143** |
| CodeContextCancelled = "TRC-0012" | ErrContextCancelled | **0330** |
| CodeAmountExceedsPrecision = "TRC-0089" | ErrAmountExceedsPrecision | **0346** |
| CodeRuleEvaluationError = "TRC-0103" | ErrRuleEvaluationFailed | **0350** |
| CodeLimitCheckError = "TRC-0136" | ErrLimitCheckFailed | **0378** |
| CodeValidationTimeout = "TRC-0229" | ErrValidationTimeout | **0422** |
| CodeListValidationsTimeout = "TRC-0252" | ErrListValidationsTimeout | **0433** |
| CodeCacheNotReady = "TRC-0281" | ErrRuleCacheNotReady | **0437** |

**tracer usage of block:** reused 16 sentinels, new 156 → highest new = **0483**. Block reserved `0328–0499` leaves `0484–0499` headroom. The 10 `Code*` HTTP consts alias existing sentinels and add 0 new codes.

---

## Entity constants (new `constant.Entity*` per family)

Append to `pkg/constant/entity.go`, alphabetized in the existing block. PascalCase value = Go domain noun.

### fees
```go
EntityPackage         = "Package"          // fee package (FEE CRUD subject)
EntityBillingPackage  = "BillingPackage"   // Motor-2 billing package
EntityFeeCalculation  = "FeeCalculation"   // fee/billing calculation execution
```

### reporter
```go
EntityTemplate   = "Template"     // report template
EntityReport     = "Report"       // generated report / output / storage object
EntityDeadline   = "Deadline"     // reporting deadline schedule
EntityDataSource = "DataSource"   // report data source / schema / extraction
```

### tracer
```go
EntityRule                  = "Rule"                  // CEL fraud/validation rule
EntityLimit                 = "Limit"                 // spending limit
EntityReservation           = "Reservation"           // two-phase limit reservation
EntityAuditEvent            = "AuditEvent"            // hash-chained audit log entry
EntityTransactionValidation = "TransactionValidation" // persisted validation record
EntityUsageCounter          = "UsageCounter"          // limit usage counter
EntityValidationRequest     = "ValidationRequest"     // inbound validation/check-limits request DTO
```

Note: tracer already shares `EntityAccount`, `EntitySegment`, `EntityPortfolio`, etc. from the canonical set where its validation request references ledger entities; only the tracer-owned domain nouns above are new. `EntityValidationRequest` is used for the inbound request-validation and check-limits codes (TRC-0089, TRC-0180..0191, TRC-0220..0237) since those validate the request DTO rather than a stored Limit/Rule.

---

## Sanity

### Counts per family (rows == sentinel count)

| family | source file | sentinel count (verified) | rows in table | reused | new | highest new code |
|---|---|---|---|---|---|---|
| fees | `components/ledger/pkg/feeshared/constant/errors.go` | 70 (`errors.New("FEE-...")`) | 70 | 16 | 54 | 0232 |
| reporter | `pkg/reporter/constant/errors.go` | 79 (59 `TPL-` + 20 `REP-`) | 79 | 18 | 61 | 0309 |
| tracer | `components/tracer/pkg/constant/errors.go` | 172 (`errors.New("TRC-...")`) + 10 `Code*` aliases | 172 + 10 alias rows | 16 | 156 | 0483 |

`rows == sentinel count` holds for all three families (70, 79, 172). Verification commands:
- fees: `grep -c '= errors.New("FEE-' …/feeshared/constant/errors.go` → 70.
- reporter TPL: `grep -c '= errors.New("TPL-' …/reporter/constant/errors.go` → 59; REP: `grep -c '= "REP-' …` → 20; total 79.
- tracer: `grep -c '= errors.New("TRC-' …/tracer/pkg/constant/errors.go` → 172; HTTP consts `grep -c '= "TRC-' …` → 10.

**Reuse breakdown (each reused canonical code keeps its existing class):**
- fees (16): 0001→0053, 0002→0009, 0003→0047, 0004→0046, 0006→0082, 0007→0077, 0008→0078, 0009→0079, 0010→0083, 0011→0080, 0012→0007, 0016→0065, 0021→0072, 0036→0081, 0041→0094, 0045→0043. → 70−16 = 54 new (0179–0232).
- reporter (18, all TPL; REP block contributes 0 reuses): 0001→0009, 0009→0065, 0011→0007, 0015→0053, 0016→0009, 0017→0047, 0018→0046, 0019→0082, 0020→0077, 0021→0078, 0022→0079, 0023→0083, 0024→0080, 0025→0081, 0026→0050, 0027→0051, 0028→0067, 0040→0084. → 79−18 = 61 new (0249–0309).
- tracer (16 sentinels): 0001→0053, 0002→0009, 0003→0047, 0004→0046, 0006→0082, 0007→0065, 0011→0143, 0020→0077, 0021→0078, 0022→0079, 0023→0083, 0040→0080, 0042→0081, 0060→0050, 0061→0051, 0062→0067. → 172−16 = 156 new (0328–0483). The 10 `Code*` aliases add 0 new codes; 3 alias reused canonical codes (CodeBadRequest→0047, CodeInternalServer→0046, CodePayloadTooLarge→0143), the other 7 alias new codes.

Total new codes allocated across all families: 54 + 61 + 156 = **271**.

### Binding allocation ranges

- **fees:** new codes `0179–0232` (54 new). Block `0179–0248`. Headroom `0233–0248`.
- **reporter:** new codes `0249–0309` (61 new). Block `0249–0327`. Headroom `0310–0327`.
- **tracer:** new codes `0328–0483` (156 new). Block `0328–0499`. Headroom `0484–0499`.
- **Next free code after all families: `0500`.**

### Collision assertions

1. No new code (`0179–0483`) collides with the canonical range `0001–0178` — all new codes start at `0179`, strictly above the canonical high-water mark `0178`. ✓
2. No new code collides across families: fees `0179–0232` < reporter `0249–0309` < tracer `0328–0483`, all disjoint with non-overlapping reserved blocks (`0179–0248`, `0249–0327`, `0328–0499`). ✓
3. All reuses target codes that already exist in `0001–0178` and keep their existing typed class — no reuse re-types an existing canonical code. (e.g. fees ErrForbiddenAccessMidaz reuses 0043 ErrInsufficientPrivileges, which is already `ForbiddenError-403`.) ✓
4. CRM-`00xx` codes live in a separate string namespace (`CRM-0006`, etc.) and do not collide with the numeric `0179–0499` block. ✓

### Status-class changes (fork-typed → binding E3 class)

All `400→422` re-types carry the `status-change` flag in their row (fork typed them as generic ValidationError/bad-request; D2 re-types business-rule/semantic violations to Unprocessable-422). Counts match the table exactly:

- **fees (8):** FEE-0015 (min>max), 0025 (flat+percentual exclusivity), 0033 (max<min), 0035 (ErrPackageRange overlap), 0037 (distribute value reconcile), 0038 (max-between-types conflict), 0043 (ErrIsDeductibleFrom chain), 0058 (ErrBillingRouteOverlap — semantic range overlap, not a 409 duplicate-id).
- **reporter (4):** TPL-0029 (ReportStatusNotFinished — state precondition), 0035 (SchemaAmbiguous), 0055 (DueDateInPast — temporal rule), 0059 (SchemaValidationFailed).
- **tracer (12):** TRC-0085 (CEL cost over threshold), 0089 (amount exceeds CEL precision), 0102 (rule status transition), 0104 (expression immutable for non-DRAFT), 0114 (rule transition not allowed), 0121 (limit status transition), 0128 (limit already DELETED), 0138 (limit immutable field), 0167 (usage counter exceeds limit), 0307 (custom period > 5y), 0308 (custom period expired), 0378 (reservation already terminal).

Infra/availability re-types — fork-generic-500 → 503 ServiceUnavailable for TRC-0012/0229/0252/0281/0328–0335, TPL-0034/0058, REP-0061 — are class refinements within the 5xx family per E3's 500/503 split, flagged in-row via the typed-class column (they do not carry the `400→422` flag).

---

## Mainline 400 reclassification (Task 3.6.1)

**Scope.** Every errorMap arm in `pkg/errors.go`'s `ValidateBusinessError` that currently produces a `ValidationError` (→ HTTP 400) **and** whose sentinel code is in the original mainline range `0001–0178`. The migrated-family blocks (`0179+`: fees, reporter, tracer) are already classified above and excluded here. CRM-prefixed sentinels (`CRM-00xx` — `ErrHolderHasInstruments`, `ErrInstrumentClosingDateBeforeCreation`, `ErrInvalidRelatedPartyRole`, `ErrRelatedPartyDocumentRequired`, `ErrRelatedPartyNameRequired`, `ErrRelatedPartyStartDateRequired`, `ErrRelatedPartyEndDateInvalid`) are also excluded — they have a `CRM-` wire prefix, not a numeric code ≤0178, and are owned by the CRM contract lock (E14).

**Standard applied (E3).** Malformed/syntactic input (bad body/UUID/date/timestamp/pagination/query/path format, missing required fields, type/enum-shape errors, unknown/unmodifiable fields) → **keep-400**. Semantic business-rule violations (state restrictions, value reconciliation, domain invariants, "cannot do X because business state Y") → **move-422** (`UnprocessableOperationError`). Conflict/duplicate-identity typed as `ValidationError` today → **move-409** (`EntityConflictError`). Genuine ambiguity biases **keep-400** (smaller wire change) and is marked `borderline`.

**Total mainline `ValidationError`-400 arms found (code ≤0178, CRM excluded): 75.** (The full mainline-block scan returns 76 `ValidationError` sentinels; `ErrMethodNotAllowed` resolves to code `0485` — a late-allocated routing code outside the ≤0178 scope — so it is excluded, leaving 75. The two reverse-misclassification entries `0017`/`0096` appear in the table as flagged notes but are NOT `ValidationError` today, so they are not part of the 75.)

| code | sentinel | current title | disposition | rationale |
|---|---|---|---|---|
| 0004 | ErrCodeUppercaseRequirement | Code Uppercase Requirement | keep-400 | format rule on a field value (uppercase) — syntactic |
| 0005 | ErrCurrencyCodeStandardCompliance | Currency Code Standard Compliance | keep-400 | ISO-4217 format check — syntactic |
| 0006 | ErrUnmodifiableField | Unmodifiable Field Error | keep-400 | request contains a non-editable field — malformed request shape |
| 0008 | ErrActionNotPermitted | Action Not Permitted | move-422 | action disallowed in current environment/state — semantic rule, not malformed input |
| 0009 | ErrMissingFieldsInRequest | Missing Fields in Request | keep-400 | missing required fields — syntactic |
| 0010 | ErrAccountTypeImmutable | Account Type Immutable | move-422 `borderline` | "type cannot be modified" is a domain immutability rule (state), not request-shape; leans 422 but adjacent to ErrImmutableField which stays 400 |
| 0011 | ErrInactiveAccountType | Inactive Account Type Error | move-422 | cannot set account type INACTIVE — business-state rule |
| 0012 | ErrAccountBalanceDeletion | Account Balance Deletion Error | move-422 | cannot delete account with remaining balance — business-state rule |
| 0013 | ErrResourceAlreadyDeleted | Resource Already Deleted | move-422 | already-deleted is a terminal-state violation, not malformed input |
| 0014 | ErrSegmentIDInactive | Segment ID Inactive | move-422 | referenced segment is inactive — business-state rule |
| 0022 | ErrImmutableField | Immutable Field Error | keep-400 `borderline` | rejects a field present in the request body before any state read — request-shape; kept 400 (contrast 0010 which gates on stored account type) |
| 0024 | ErrAccountStatusTransactionRestriction | Account Status Transaction Restriction | move-422 | account status does not permit the transaction — business-state rule (fires in transaction validation, `pkg/mtransaction/validations.go`) |
| 0025 | ErrInsufficientAccountBalance | Insufficient Account Balance Error | move-422 | insufficient balance — financial business-rule (semantic sibling of ErrInsufficientFunds 0018, already 422) |
| 0026 | ErrTransactionMethodRestriction | Transaction Method Restriction | move-422 | method not permitted for these accounts — business-rule |
| 0029 | ErrInvalidParentAccountID | Invalid Parent Account ID | keep-400 `borderline` | message says the parent account "does not exist"; reads like a 404, but typed as a request-field validation. Kept 400 (smaller change); flag for owner — arguably EntityNotFound-404 |
| 0030 | ErrMismatchedAssetCode | Mismatched Asset Code | move-422 | parent account's asset code conflicts with the requested one — cross-entity consistency rule, not malformed input |
| 0031 | ErrChartTypeNotFound | Chart Type Not Found | keep-400 `borderline` | "chart type does not exist" — enum-membership check on an input value; kept 400 (enum-shape), not a stored-entity 404 |
| 0032 | ErrInvalidCountryCode | Invalid Country Code | keep-400 | ISO-3166 format check — syntactic |
| 0033 | ErrInvalidCodeFormat | Invalid Code Format | keep-400 | alphanumeric/case format rule — syntactic |
| 0040 | ErrInvalidType | Invalid Type | keep-400 | enum-shape check on `type` — syntactic |
| 0048 | ErrInvalidDSLFileFormat | Invalid DSL File Format | keep-400 | malformed DSL file structure/syntax — syntactic |
| 0049 | ErrEmptyDSLFile | Empty DSL File | keep-400 | empty uploaded file — malformed input |
| 0050 | ErrMetadataKeyLengthExceeded | Metadata Key Length Exceeded | keep-400 | length constraint on input — syntactic |
| 0051 | ErrMetadataValueLengthExceeded | Metadata Value Length Exceeded | keep-400 | length constraint on input — syntactic |
| 0065 | ErrInvalidPathParameter | Invalid Path Parameter | keep-400 | malformed path param format — syntactic |
| 0066 | ErrInvalidAccountType | Invalid Account Type | keep-400 | enum-shape check — syntactic |
| 0067 | ErrInvalidMetadataNesting | Invalid Metadata Nesting | keep-400 | nested metadata is a structural-shape violation — syntactic |
| 0072 | ErrInvalidTransactionType | Invalid Transaction Type | keep-400 | exactly-one-of field-shape rule, fires in body validation (`withBody.go`) — syntactic |
| 0073 | ErrTransactionValueMismatch | Transaction Value Mismatch | move-422 | source/destination sums do not reconcile to the amount — the canonical E3 example of a semantic rule mis-typed as 400 (`error-handling.md` E3) |
| 0074 | ErrForbiddenExternalAccountManipulation | External Account Modification Prohibited | move-422 | external accounts cannot be modified/deleted — domain rule on entity kind, not malformed input |
| 0077 | ErrInvalidDateFormat | Invalid Date Format Error | keep-400 | yyyy-mm-dd format check — syntactic |
| 0078 | ErrInvalidFinalDate | Invalid Final Date Error | keep-400 `borderline` | finalDate < initialDate is a range rule but on raw query params at parse time; kept 400 (pagination/date-param family stays 400 for consistency) |
| 0079 | ErrDateRangeExceedsLimit | Date Range Exceeds Limit Error | keep-400 | query date-range bound — pagination/query family, syntactic |
| 0080 | ErrPaginationLimitExceeded | Pagination Limit Exceeded | keep-400 | pagination bound — syntactic |
| 0081 | ErrInvalidSortOrder | Invalid Sort Order | keep-400 | enum-shape on sort_order — syntactic |
| 0082 | ErrInvalidQueryParameter | Invalid Query Parameter | keep-400 | malformed query param — syntactic |
| 0083 | ErrInvalidDateRange | Invalid Date Range Error | keep-400 | required date fields/format — syntactic |
| 0086 | ErrLockVersionAccountBalance | Race condition detected | move-422 `borderline` | optimistic-lock/version conflict — a concurrency state condition, not malformed input. Sibling ErrStaleBalanceVersion (0174) is already 422; 409 is also defensible (conflict). Recommend 422 to match the sibling; flag for owner |
| 0087 | ErrTransactionIDHasAlreadyParentTransaction | Transaction Revert already exist | move-422 | revert already exists — terminal-state rule on the transaction (fires in RevertTransaction) |
| 0088 | ErrTransactionIDIsAlreadyARevert | Transaction is already a reversal | move-422 | transaction is already a reversal — state rule, cannot revert a revert |
| 0089 | ErrTransactionCantRevert | Transaction can't be reverted | move-422 | transaction not in a revertable state — business-state rule |
| 0090 | ErrTransactionAmbiguous | Transaction ambiguous account | move-422 `borderline` | same account in sources+destinations — semantic accounting rule; fires in ValidateSendSourceAndDistribute. Leans 422 (double-entry invariant), though arguably request-shape; recommend 422 |
| 0091 | ErrParentIDSameID | ID cannot be used as the parent ID | move-422 `borderline` | self-referential parent — relational invariant on the entity graph; could read as field-shape (400). Recommend 422 (it depends on the entity identity, not just syntax) |
| 0093 | ErrBalancesCantBeDeleted | Balance cannot be deleted | move-422 | cannot delete a balance with funds — business-state rule (sibling of ErrBalanceRemainingDeletion 0016, already 422) |
| 0096 | ErrAccountAliasInvalid | Invalid Account Alias | keep-400 `borderline` | alias contains invalid characters → syntactic; **note misclassification**: currently typed `InternalServerError`-500, NOT ValidationError — see flag list below (excluded from the 75 count) |
| 0103 | ErrInvalidOperationRouteType | Invalid Operation Route Type | keep-400 | enum-shape on operationType — syntactic |
| 0104 | ErrMissingOperationRoutes | Missing Operation Routes in Request | keep-400 `borderline` | "must include at least one of each type" — could be a composition rule (422) but reads as a required-fields completeness check at request validation. Kept 400 (missing-fields family) |
| 0111 | ErrInvalidAccountRuleType | Invalid Account Rule Type | keep-400 | enum-shape on ruleType — syntactic |
| 0112 | ErrInvalidAccountRuleValue | Invalid Account Rule Value | keep-400 | type-shape on validIf (string vs array) — syntactic |
| 0115 | ErrInvalidTransactionRouteID | Invalid Transaction Route ID | keep-400 | malformed UUID — syntactic |
| 0120 | ErrInvalidAccountTypeKeyValue | Invalid Characters | keep-400 | invalid-char check on keyValue — syntactic |
| 0121 | ErrInvalidFutureTransactionDate | Invalid Future Date Error | keep-400 `borderline` | "transactionDate cannot be future" is a temporal rule; but it validates a single input field against now() at request validation — kept 400 (date-field family). Flag: sibling temporal rules in migrated families (e.g. DueDateInPast) went 422 |
| 0122 | ErrInvalidPendingFutureTransactionDate | Invalid Field for Pending Transaction Error | keep-400 | field not supported for pending mode — request-shape conditional-field rule |
| 0124 | ErrAdditionalBalanceNotAllowed | Additional Balance Creation Not Allowed | move-422 | additional balances not allowed for external account type — domain rule on entity kind |
| 0131 | ErrInvalidDatetimeFormat | Invalid Datetime Format Error | keep-400 | datetime format check — syntactic |
| 0134 | ErrMetadataIndexInvalidKey | Invalid Metadata Key Format | keep-400 | key format rule — syntactic |
| 0135 | ErrMetadataIndexLimitExceeded | Metadata Index Limit Exceeded | move-422 `borderline` | max-indexes-reached is a resource/state cap, not malformed input. Recommend 422; small surface (CRM/metadata-index endpoint). Flag for owner |
| 0137 | ErrMetadataIndexDeletionForbidden | Metadata Index Deletion Forbidden | move-422 | system indexes cannot be deleted — domain rule on entity kind (not auth/403, not malformed input) |
| 0138 | ErrInvalidEntityName | Invalid Entity Name | keep-400 | entity-name shape check — syntactic |
| 0140 | ErrInvalidTimestamp | Invalid Timestamp | keep-400 `borderline` | "timestamp cannot be in the future" — temporal field check at request validation; kept 400 (timestamp-field family, same call as 0121) |
| 0142 | ErrMissingRequiredQueryParameter | Missing Required Query Parameter | keep-400 | missing query param — syntactic |
| 0143 | ErrPayloadTooLarge | Payload Too Large | keep-400 | request size bound — syntactic (HTTP 413-adjacent; stays in 400 family per current typing) |
| 0144 | ErrJSONNestingDepthExceeded | JSON Nesting Depth Exceeded | keep-400 | structural bound on payload — syntactic |
| 0145 | ErrJSONKeyCountExceeded | JSON Key Count Exceeded | keep-400 | structural bound on payload — syntactic |
| 0147 | ErrUnknownSettingsField | Unknown Settings Field | keep-400 | unknown field in body — syntactic |
| 0148 | ErrInvalidSettingsFieldType | Invalid Settings Field Type | keep-400 | type-shape on settings field — syntactic |
| 0149 | ErrSettingsRootLevelField | Settings Field at Root Level | keep-400 | field-nesting structural rule — syntactic |
| 0153 | ErrNoSourceForAction | No Source for Action | keep-400 `borderline` | action requires a source route — route-composition completeness; sibling ErrNoRoutesForAction (0157) is 422. Borderline 422, kept 400 (validates the submitted route payload shape). Flag for owner |
| 0154 | ErrNoDestinationForAction | No Destination for Action | keep-400 `borderline` | mirror of 0153 — same reasoning, flag for owner |
| 0155 | ErrInvalidRouteAction | Invalid Route Action | keep-400 | enum-shape on action — syntactic |
| 0156 | ErrDuplicateActionRoute | Duplicate Action Route | move-409 `borderline` | "operation route already assigned to action" — duplicate-within-payload. Conflict semantics → 409. Borderline: it is a within-request duplicate (not a stored-identity collision), so 422 is also defensible; recommend 409 to match the conflict class, flag for owner |
| 0158 | ErrTooManyOperationRoutes | Too Many Operation Routes | keep-400 | count bound on operation routes — syntactic (size/cardinality of the submitted payload) |
| 0176 | ErrInvalidSettingsFieldValue | Invalid Settings Field Value | keep-400 | allowed-values enum check — syntactic |
| 0170 | ErrReservedBalanceKey | Reserved Balance Key Error | move-422 `borderline` | the supplied balance key is reserved for system use — a namespace/domain rule, not a malformed value. Leans 422; kept-400 also defensible (input-validation). Recommend 422 |
| 0171 | ErrInvalidBalanceDirection | Invalid Balance Direction Error | keep-400 | enum-shape ("credit"/"debit") — syntactic |
| 0172 | ErrInvalidBalanceSettings | Invalid Balance Settings Error | keep-400 | malformed settings payload (overdraft/limit/scope) — syntactic |
| 0017 | ErrInvalidScriptFormat | Invalid Script Format Error | keep-400 `borderline` | **note misclassification**: currently typed `EntityConflictError`-409, NOT ValidationError. Message is a DSL parse/format error → should be 400. Excluded from the 75 count; see flag list below |

### Disposition counts (of the 75 ValidationError-400 arms)

- **keep-400: 51**
- **move-422: 23** — 0008, 0010, 0011, 0012, 0013, 0014, 0024, 0025, 0026, 0030, 0073, 0074, 0086, 0087, 0088, 0089, 0090, 0091, 0093, 0124, 0135, 0137, 0170
- **move-409: 1** — 0156 (ErrDuplicateActionRoute)
- of which `borderline`-flagged: **16** (0010, 0022, 0029, 0031, 0078, 0086, 0090, 0091, 0104, 0121, 0135, 0140, 0153, 0154, 0156, 0170)

(51 + 23 + 1 = 75. ✓)

### Reverse-misclassification flags (codes ≤0178 NOT typed ValidationError that look syntactic — flag-only, no disposition)

These are currently typed `UnprocessableOperationError`/`EntityConflictError`/`InternalServerError` but their title/message reads syntactic (malformed input), the opposite of the 400→422 drift. Surfaced for owner review; not part of the 75-arm re-typing:

- **0017 ErrInvalidScriptFormat** — typed `EntityConflictError`-409. A DSL parse/format failure is malformed input → looks like it should be **400**, not 409. The 409 class is almost certainly a historical mistake (a parse error is not a resource conflict).
- **0096 ErrAccountAliasInvalid** — typed `InternalServerError`-500. Title/message ("The alias contains invalid characters") is plainly client-supplied malformed input → should be **400**, never 500. A 500 here leaks an input-validation failure as a server fault (also brushes E9).

No syntactic-looking arms were found mis-typed as 422 among the accounting/transaction `UnprocessableOperationError` set — those (0098, 0099, 0107, 0113, 0114, 0116–0119, 0150–0152, 0157, 0162–0169, 0173–0175, 0177, 0178) are all genuine semantic/state rules and correctly 422.
