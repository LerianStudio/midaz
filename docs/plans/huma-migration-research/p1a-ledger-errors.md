# P1a — LEDGER Error Contract Inventory

Worktree: `.../scratchpad/midaz-consolidation`
Sources of truth:
- Envelopes: `pkg/errors.go`, `pkg/mmodel/error.go`
- Codes: `pkg/constant/errors.go`
- Code→envelope→title/message: `pkg/errors.go` (`ValidateBusinessError`)
- Envelope-type→HTTP status: `pkg/net/http/errors.go` (`WithError`), `pkg/net/http/response.go`

Distinct error codes: **422** (394 numeric `00xx`, highest `0491`; 28 `CRM-00xx`).

---

## 1. Error Envelope Shapes

### 1.1 Wire response envelope — `mmodel.Error` (`pkg/mmodel/error.go:11`)

This is what the client actually receives on the wire (swagger `@name Error`). It is NOT the same struct the domain layer throws; the fiber handlers project the internal typed errors down to this shape via `fiber.Map` in `pkg/net/http/response.go`.

| Field | Go type | JSON tag | Notes |
|-------|---------|----------|-------|
| Code | `string` | `json:"code"` (required) | `maxLength:50`, example `ERR_INVALID_INPUT` — but real values are `"0147"`, `"CRM-0006"`, etc. |
| Title | `string` | `json:"title"` (required) | `maxLength:100`, example `Bad Request` |
| Message | `string` | `json:"message"` (required) | `maxLength:500` |
| EntityType | `string` | `json:"entityType,omitempty"` | `maxLength:100`, example `Organization` |
| Fields | `map[string]string` | `json:"fields,omitempty"` | field→message validation detail |

`ErrorResponse` (`error.go:41`) wraps `Body Error` for swagger only.

Note: many response paths do NOT emit the full struct. `response.go` helpers (`NotFound`, `Conflict`, `UnprocessableEntity`, `BadRequest`, `Unauthorized`, `Forbidden`, `InternalServerError`, `ServiceUnavailable`) serialize a bare `fiber.Map{"code","title","message"}` — no `entityType`, no `fields`. `fields` only reaches the wire when the handler passes a `ValidationKnownFieldsError`/`ValidationUnknownFieldsError` directly to `BadRequest` (400).

### 1.2 Internal typed errors (`pkg/errors.go`)

Eleven typed error structs. Ten share an identical 5-field shape; two validation types add a `Fields` map. Each field is the same across the shared set:

Shared shape (`EntityNotFoundError`, `ValidationError`, `EntityConflictError`, `UnauthorizedError`, `ForbiddenError`, `UnprocessableOperationError`, `HTTPError`, `FailedPreconditionError`, `ServiceUnavailableError`, `InternalServerError`, `ResponseError`):

| Field | Go type | JSON tag |
|-------|---------|----------|
| EntityType | `string` | `json:"entityType,omitempty"` |
| Title | `string` | `json:"title,omitempty"` |
| Message | `string` | `json:"message,omitempty"` |
| Code | `string` | `json:"code,omitempty"` |
| Err | `error` | `json:"err,omitempty"` |

**`HTTPError`** (`pkg/errors.go:140`) — exactly this shape (`entityType`, `title`, `message`, `code`, `err`). This is the `pkg.HTTPError{code,title,message,entityType,err}` named in the task. `Error()` returns `Message`.

Field-bearing variants (extra last field):

| Type | Extra field | Go type | JSON tag |
|------|-------------|---------|----------|
| `ValidationKnownFieldsError` (`:209`) | Fields | `FieldValidations` = `map[string]string` | `json:"fields,omitempty"` |
| `ValidationUnknownFieldsError` (`:229`) | Fields | `UnknownFields` = `map[string]any` | `json:"fields,omitempty"` |

`FieldValidations` (`:227`) = `map[string]string`; `UnknownFields` (`:248`) = `map[string]any`.

`Error()` string behavior differs by type:
- `ValidationError`, `UnprocessableOperationError`: `"<code> - <message>"` when `Code` set, else `Message`.
- `EntityNotFoundError`: `Message`, else `"Entity <EntityType> not found"`, else wrapped err, else `"entity not found"`.
- `EntityConflictError`: `Message`, else wrapped err.
- All others (`Unauthorized`, `Forbidden`, `HTTPError`, `FailedPrecondition`, `ServiceUnavailable`, `InternalServer`, `ResponseError`, both field variants): return `Message`.

`IsBusinessError(err)` (`:254`) classifies NotFound/Validation/Conflict/Unauthorized/Forbidden/Unprocessable/KnownFields/UnknownFields as business (expected) errors via `errors.As`.

---

## 2. Envelope-Type → HTTP Status Mapping (`pkg/net/http/errors.go:WithError`)

`WithError` resolves via `errors.As` in **declaration order** — first match wins, and every platform type has `Unwrap`, so wrapped errors still classify. Do not nest platform errors (outermost wins).

| Typed error | HTTP status | Emits `fields`? |
|-------------|-------------|-----------------|
| `EntityNotFoundError` | **404** Not Found | no |
| `EntityConflictError` | **409** Conflict | no |
| `ValidationError` | **400** Bad Request (repacked as `ValidationKnownFieldsError` with `fields:nil`) | no (nil) |
| `UnprocessableOperationError` | **422** Unprocessable Entity | no |
| `UnauthorizedError` | **401** Unauthorized | no |
| `ForbiddenError` | **403** Forbidden | no |
| `ValidationKnownFieldsError` | **400** Bad Request | yes |
| `ValidationUnknownFieldsError` | **400** Bad Request | yes |
| `ResponseError` | `strconv.Atoi(err.Code)` — status IS the numeric code field | passes full struct |
| `libCommons.Response` | see below | default→400 |
| `InternalServerError` | **500** | no |
| `FailedPreconditionError` | **500** (note: mapped to 500, not 412) | no |
| `ServiceUnavailableError` | **503** | no |
| (no match / fallthrough) | **500** via `ValidateInternalError(err,"")` → code `0046` | no |

`libCommons.Response` sub-switch (lib-commons codes, not the constant/errors.go set):
- `ErrInsufficientFunds`, `ErrAccountIneligibility` → 422
- `ErrAssetCodeNotFound` → 404
- `ErrOverFlowInt64` → 500
- default → 400

**Caveat**: `ResponseError` is the odd one — its `Code` is parsed as the HTTP status integer (`JSONResponseError`, `response.go:123`), so `ResponseError` instances carry an HTTP status in `Code`, not an `00xx` business code. Produced by `ValidateUnmarshallingError` (code `0094`) and `ValidateInternalError`-adjacent paths — check callers before assuming `Code` is an `00xx`.

Because status is driven purely by the **Go type** chosen in `ValidateBusinessError` (not the code number), the code→status column in section 4 is derived by joining each code's assigned type here.

---

## 3. Code Structure & Gaps

- **Numeric block** `0001`–`0491` (defined as `errors.New("00xx")`), highest `0491`.
- Numeric gaps (unallocated / reserved / migrated-away): `0016`, `0130`, `0156`, and the contiguous `0234`–`0327` (94 codes) reserved between the Fee block (ends `0233`) and the Tracer block (starts `0328`).
- **Fee platform block**: `0179`–`0233` (migrated from `FEE-xxxx`).
- **Tracer platform block**: `0328`–`0491` (migrated from `TRC-xxxx`; original comment says 0328-0499 highest allocated 0483, but 0484-0491 were subsequently added: route/method/pending-lock/reservation-tenant/instrument-ref/skip/holder).
- **CRM block**: `CRM-0006`,`0008`,`0010`,`0013`,`0017`,`0019`–`0041` (28 codes; sequence has intentional gaps — independent sentinels, not an array). Keyset/registry/audit family = `CRM-0031`–`CRM-0041`.

Sentinels defined but NOT in the `ValidateBusinessError` map (18) — handled by other paths or not surfaced through that mapper:
`ErrInternalServer` (0046, via `ValidateInternalError`), `ErrBadRequest` (0047, via `ValidateBadRequestFieldsError`), `ErrUnexpectedFieldsInTheRequest` (0053, via `ValidateBadRequestFieldsError`), `ErrInvalidRequestBody` (0094, via `ValidateUnmarshallingError`), `ErrNoBalancesFound` (0092), `ErrTenantNotProvisioned` (0146), `ErrTenantServiceSuspended` (0159), `ErrTenantNotFound` (0160), `ErrTenantServiceUnavailable` (0161), `ErrMetadataQueryInvalidFormat` (CRM-0019), `ErrMetadataQueryInvalidKey` (CRM-0020), `ErrMetadataQueryContainsOperator` (CRM-0021), `ErrInvalidHeaderValue` (CRM-0022), `ErrKeysetNotFound` (CRM-0031), `ErrKeysetAlreadyExists` (CRM-0032), `ErrKeysetRevisionConflict` (CRM-0033), `ErrRegistryRevisionConflict` (CRM-0036), `ErrAuditWriteFailed` (CRM-0040). Their code strings still exist and are wire-valid; they are just not routed through the central business-error mapper.

`MissingFieldsInRequest` (0009) appears in BOTH `ValidateBadRequestFieldsError` (as `ValidationKnownFieldsError`, "Missing Fields in Request" with `fields`) and `ValidateBusinessError` (as plain `ValidationError`). `Bad Request` (0047) / `Unexpected Fields` (0053) titles+messages live only in `ValidateBadRequestFieldsError`.

---

## 4. Full Code Taxonomy

Columns: Code | Sentinel (`constant.*`) | Envelope type | HTTP status (derived from §2) | Title | Message (fmt verbs indicate runtime-interpolated args).

Status legend: 400=ValidationError/KnownFields, 404=EntityNotFound, 409=EntityConflict, 422=UnprocessableOperation, 401=Unauthorized, 403=Forbidden, 500=InternalServer/FailedPrecondition, 503=ServiceUnavailable.

### 4.1 Core numeric codes (mapped in `ValidateBusinessError`)

| Code | Sentinel | Type | Status | Title | Message |
|------|----------|------|--------|-------|---------|
| 0001 | ErrDuplicateLedger | Conflict | 409 | Duplicate Ledger Error | A ledger with the name %v already exists in the division %v… |
| 0002 | ErrLedgerNameConflict | Conflict | 409 | Ledger Name Conflict | A ledger named %v already exists in your organization… |
| 0003 | ErrAssetNameOrCodeDuplicate | Conflict | 409 | Asset Name or Code Duplicate | An asset with the same name or code already exists… |
| 0004 | ErrCodeUppercaseRequirement | Validation | 400 | Code Uppercase Requirement | The code must be in uppercase… |
| 0005 | ErrCurrencyCodeStandardCompliance | Validation | 400 | Currency Code Standard Compliance | Currency-type assets must comply with the ISO-4217 standard… |
| 0006 | ErrUnmodifiableField | Validation | 400 | Unmodifiable Field Error | Your request includes a field that cannot be modified… |
| 0007 | ErrEntityNotFound | NotFound | 404 | Entity Not Found | No entity was found for the given ID… |
| 0008 | ErrActionNotPermitted | Unprocessable | 422 | Action Not Permitted | The action you are attempting is not allowed in the current environment… |
| 0009 | ErrMissingFieldsInRequest | Validation (also KnownFields in ValidateBadRequestFieldsError) | 400 | Missing Fields in Request | Your request is missing one or more required fields: %v… |
| 0010 | ErrAccountTypeImmutable | Unprocessable | 422 | Account Type Immutable | The account type specified cannot be modified… |
| 0011 | ErrInactiveAccountType | Unprocessable | 422 | Inactive Account Type Error | The account type specified cannot be set to INACTIVE… |
| 0012 | ErrAccountBalanceDeletion | Unprocessable | 422 | Account Balance Deletion Error | An account or sub-account cannot be deleted if it has a remaining balance… |
| 0013 | ErrResourceAlreadyDeleted | Unprocessable | 422 | Resource Already Deleted | The resource you are trying to delete has already been deleted… |
| 0014 | ErrSegmentIDInactive | Unprocessable | 422 | Segment ID Inactive | The Segment ID you are attempting to use is inactive… |
| 0015 | ErrDuplicateSegmentName | Conflict | 409 | Duplicate Segment Name Error | A segment with the name %v already exists for this ledger ID %v… |
| 0017 | ErrInvalidScriptFormat | Validation | 400 | Invalid Script Format Error | The script provided in your request is invalid… |
| 0018 | ErrInsufficientFunds | Unprocessable | 422 | Insufficient Funds Error | The transaction could not be completed due to insufficient funds… |
| 0019 | ErrAccountIneligibility | Unprocessable | 422 | Account Ineligibility Error | One or more accounts listed in the transaction are not eligible… |
| 0020 | ErrAliasUnavailability | Conflict | 409 | Alias Unavailability Error | The alias %v is already in use… |
| 0021 | ErrParentTransactionIDNotFound | NotFound | 404 | Parent Transaction ID Not Found | The parentTransactionId %v does not correspond to any existing transaction… |
| 0022 | ErrImmutableField | Validation | 400 | Immutable Field Error | The %v field cannot be modified… |
| 0023 | ErrTransactionTimingRestriction | Unprocessable | 422 | Transaction Timing Restriction | You can only perform another transaction using %v of %f from %v to %v after %v… |
| 0024 | ErrAccountStatusTransactionRestriction | Unprocessable | 422 | Account Status Transaction Restriction | The current statuses of the source and/or destination accounts do not permit transactions… |
| 0025 | ErrInsufficientAccountBalance | Unprocessable | 422 | Insufficient Account Balance Error | The account %v does not have sufficient balance… |
| 0026 | ErrTransactionMethodRestriction | Unprocessable | 422 | Transaction Method Restriction | Transactions involving %v are not permitted… |
| 0027 | ErrDuplicateTransactionTemplateCode | Conflict | 409 | Duplicate Transaction Template Code Error | A transaction template with the code %v already exists… |
| 0028 | ErrDuplicateAssetPair | Conflict | 409 | Duplicate Asset Pair Error | A pair for the assets %v%v already exists with the ID %v… |
| 0029 | ErrInvalidParentAccountID | Validation | 400 | Invalid Parent Account ID | The specified parent account ID does not exist… |
| 0030 | ErrMismatchedAssetCode | Unprocessable | 422 | Mismatched Asset Code | The parent account ID you provided is associated with a different asset code… |
| 0031 | ErrChartTypeNotFound | Validation | 400 | Chart Type Not Found | The chart type %v does not exist… |
| 0032 | ErrInvalidCountryCode | Validation | 400 | Invalid Country Code | …'address.country' … ISO-3166 alpha-2 standard… |
| 0033 | ErrInvalidCodeFormat | Validation | 400 | Invalid Code Format | The 'code' field must be alphanumeric, in upper case… |
| 0034 | ErrAssetCodeNotFound | NotFound | 404 | Asset Code Not Found | The provided asset code does not exist in our records… |
| 0035 | ErrPortfolioIDNotFound | NotFound | 404 | Portfolio ID Not Found | The provided portfolio ID does not exist… |
| 0036 | ErrSegmentIDNotFound | NotFound | 404 | Segment ID Not Found | The provided segment ID does not exist… |
| 0037 | ErrLedgerIDNotFound | NotFound | 404 | Ledger ID Not Found | The provided ledger ID does not exist… |
| 0038 | ErrOrganizationIDNotFound | NotFound | 404 | Organization ID Not Found | The provided organization ID does not exist… |
| 0039 | ErrParentOrganizationIDNotFound | NotFound | 404 | Parent Organization ID Not Found | The provided parent organization ID does not exist… |
| 0040 | ErrInvalidType | Validation | 400 | Invalid Type | …Accepted types are currency, crypto, commodities, or others… |
| 0041 | ErrTokenMissing | Unauthorized | 401 | Token Missing | A valid token must be provided in the request header… |
| 0042 | ErrInvalidToken | Unauthorized | 401 | Invalid Token | The provided token is expired, invalid or malformed… |
| 0043 | ErrInsufficientPrivileges | Forbidden | 403 | Insufficient Privileges | You do not have the necessary permissions… |
| 0044 | ErrPermissionEnforcement | FailedPrecondition | 500 | Permission Enforcement Error | The enforcer is not configured properly… |
| 0045 | ErrJWKFetch | FailedPrecondition | 500 | JWK Fetch Error | The JWK keys could not be fetched from the source… |
| 0046 | ErrInternalServer | InternalServer (via ValidateInternalError) | 500 | Internal Server Error | The server encountered an unexpected error… |
| 0047 | ErrBadRequest | KnownFields (via ValidateBadRequestFieldsError) | 400 | Bad Request | The server could not understand the request due to malformed syntax… |
| 0048 | ErrInvalidDSLFileFormat | Validation | 400 | Invalid DSL File Format | The submitted DSL file %v is in an incorrect format… |
| 0049 | ErrEmptyDSLFile | Validation | 400 | Empty DSL File | The submitted DSL file %v is empty… |
| 0050 | ErrMetadataKeyLengthExceeded | Validation | 400 | Metadata Key Length Exceeded | The metadata key %v exceeds the maximum allowed length of %v… |
| 0051 | ErrMetadataValueLengthExceeded | Validation | 400 | Metadata Value Length Exceeded | The metadata value %v exceeds the maximum allowed length of %v… |
| 0052 | ErrAccountIDNotFound | NotFound | 404 | Account ID Not Found | The provided account ID does not exist… |
| 0053 | ErrUnexpectedFieldsInTheRequest | UnknownFields (via ValidateBadRequestFieldsError) | 400 | Unexpected Fields in the Request | The request body contains more fields than expected… |
| 0054 | ErrIDsNotFoundForAccounts | NotFound | 404 | IDs Not Found for Accounts | No accounts were found for the provided IDs… |
| 0055 | ErrAssetIDNotFound | NotFound | 404 | Asset ID Not Found | The provided asset ID does not exist… |
| 0056 | ErrNoAssetsFound | NotFound | 404 | No Assets Found | No assets were found in the search… |
| 0057 | ErrNoSegmentsFound | NotFound | 404 | No Segments Found | No segments were found in the search… |
| 0058 | ErrNoPortfoliosFound | NotFound | 404 | No Portfolios Found | No portfolios were found in the search… |
| 0059 | ErrNoOrganizationsFound | NotFound | 404 | No Organizations Found | No organizations were found in the search… |
| 0060 | ErrNoLedgersFound | NotFound | 404 | No Ledgers Found | No ledgers were found in the search… |
| 0061 | ErrBalanceUpdateFailed | NotFound | 404 | Balance Update Failed | The balance could not be updated for the specified account ID… |
| 0062 | ErrNoAccountIDsProvided | NotFound | 404 | No Account IDs Provided | No account IDs were provided for the balance update… |
| 0063 | ErrFailedToRetrieveAccountsByAliases | NotFound | 404 | Failed To Retrieve Accounts By Aliases | The accounts could not be retrieved using the specified aliases… |
| 0064 | ErrNoAccountsFound | NotFound | 404 | No Accounts Found | No accounts were found in the search… |
| 0065 | ErrInvalidPathParameter | Validation | 400 | Invalid Path Parameter | One or more path parameters are in an incorrect format %v… |
| 0066 | ErrInvalidAccountType | Validation | 400 | Invalid Account Type | The provided 'type' is not valid. |
| 0067 | ErrInvalidMetadataNesting | Validation | 400 | Invalid Metadata Nesting | The metadata object cannot contain nested values %v… |
| 0068 | ErrOperationIDNotFound | NotFound | 404 | Operation ID Not Found | The provided operation ID does not exist… |
| 0069 | ErrNoOperationsFound | NotFound | 404 | No Operations Found | No operations were found in the search… |
| 0070 | ErrTransactionIDNotFound | NotFound | 404 | Transaction ID Not Found | The provided transaction ID does not exist… |
| 0071 | ErrNoTransactionsFound | NotFound | 404 | No Transactions Found | No transactions were found in the search… |
| 0072 | ErrInvalidTransactionType | Validation | 400 | Invalid Transaction Type | Only one transaction type ('amount','share','remaining') must be specified in '%v'… |
| 0073 | ErrTransactionValueMismatch | Unprocessable | 422 | Transaction Value Mismatch | The values for the source, the destination, or both do not match… |
| 0074 | ErrForbiddenExternalAccountManipulation | Unprocessable | 422 | External Account Modification Prohibited | Accounts of type 'external' cannot be deleted or modified… |
| 0075 | ErrAuditRecordNotRetrieved | NotFound | 404 | Audit Record Not Retrieved | The record %v could not be retrieved for audit… |
| 0076 | ErrAuditTreeRecordNotFound | NotFound | 404 | Audit Tree Record Not Found | The record %v does not exist in the audit tree… |
| 0077 | ErrInvalidDateFormat | Validation | 400 | Invalid Date Format Error | The 'initialDate','finalDate'… use 'yyyy-mm-dd'… |
| 0078 | ErrInvalidFinalDate | Validation | 400 | Invalid Final Date Error | The 'finalDate' cannot be earlier than the 'initialDate'… |
| 0079 | ErrDateRangeExceedsLimit | Validation | 400 | Date Range Exceeds Limit Error | …exceeds the permitted limit of %v months… |
| 0080 | ErrPaginationLimitExceeded | Validation | 400 | Pagination Limit Exceeded | …maximum allowed of %v items per page… |
| 0081 | ErrInvalidSortOrder | Validation | 400 | Invalid Sort Order | The 'sort_order' field must be 'asc' or 'desc'… |
| 0082 | ErrInvalidQueryParameter | Validation | 400 | Invalid Query Parameter | One or more query parameters are in an incorrect format '%v'… |
| 0083 | ErrInvalidDateRange | Validation | 400 | Invalid Date Range Error | Both 'initialDate' and 'finalDate' are required, 'yyyy-mm-dd'… |
| 0084 | ErrIdempotencyKey | Conflict | 409 | Duplicate Idempotency Key | The idempotency key %v is already in use… |
| 0085 | ErrAccountAliasNotFound | NotFound | 404 | Account Alias Not Found | The provided account Alias does not exist… |
| 0086 | ErrLockVersionAccountBalance | Unprocessable | 422 | Race condition detected | A race condition was detected while processing your request… |
| 0087 | ErrTransactionIDHasAlreadyParentTransaction | Conflict | 409 | Transaction Revert already exist | Transaction revert already exists… |
| 0088 | ErrTransactionIDIsAlreadyARevert | Conflict | 409 | Transaction is already a reversal | Transaction is already a reversal… |
| 0089 | ErrTransactionCantRevert | Unprocessable | 422 | Transaction can't be reverted | Transaction can't be reverted… |
| 0090 | ErrTransactionAmbiguous | Unprocessable | 422 | Transaction ambiguous account | Transaction can't use the same account in sources and destinations |
| 0091 | ErrParentIDSameID | Unprocessable | 422 | ID cannot be used as the parent ID | The provided ID cannot be used as the parent ID… |
| 0092 | ErrNoBalancesFound | (unmapped) | — | — | (code defined; not in ValidateBusinessError) |
| 0093 | ErrBalancesCantBeDeleted | Conflict | 409 | Balance cannot be deleted | Balance cannot be deleted because it still has funds in it. |
| 0094 | ErrInvalidRequestBody | ResponseError (via ValidateUnmarshallingError) | status=Code | Unmarshalling error | invalid value for field … / err.Error() |
| 0095 | ErrMessageBrokerUnavailable | InternalServer | 500 | Message Broker Unavailable | …unexpected error while connecting to Message Broker… |
| 0096 | ErrAccountAliasInvalid | Validation | 400 | Invalid Account Alias | The alias contains invalid characters… |
| 0097 | ErrOverFlowInt64 | InternalServer | 500 | Overflow Error | …could not be completed due to an overflow… |
| 0098 | ErrOnHoldExternalAccount | Unprocessable | 422 | Invalid Pending Transaction | External accounts cannot be used for pending transactions in source operations… |
| 0099 | ErrCommitTransactionNotPending | Conflict | 409 | Invalid Transaction Status | The transaction status does not allow the requested action… |
| 0100 | ErrOperationRouteTitleAlreadyExists | Conflict | 409 | Operation Route Title Already Exists | The 'title' provided already exists for the 'type' provided… |
| 0101 | ErrOperationRouteNotFound | NotFound | 404 | Operation Route Not Found | The provided operation route does not exist… |
| 0102 | ErrNoOperationRoutesFound | NotFound | 404 | No Operation Routes Found | No operation routes were found in the search… |
| 0103 | ErrInvalidOperationRouteType | Validation | 400 | Invalid Operation Route Type | …'source','destination','bidirectional'… |
| 0104 | ErrMissingOperationRoutes | Validation | 400 | Missing Operation Routes in Request | …at least one operation route of each type (debit and credit)… |
| 0105 | ErrTransactionRouteNotFound | NotFound | 404 | Transaction Route Not Found | The provided transaction route does not exist… |
| 0106 | ErrNoTransactionRoutesFound | NotFound | 404 | No Transaction Routes Found | No transaction routes were found in the search… |
| 0107 | ErrOperationRouteLinkedToTransactionRoutes | Unprocessable | 422 | Operation Route Linked to Transaction Routes | …cannot be deleted because it is linked to one or more transaction routes… |
| 0108 | ErrDuplicateAccountTypeKeyValue | Conflict | 409 | Duplicate Account Type Key Value Error | An account type with the specified key value already exists… |
| 0109 | ErrAccountTypeNotFound | NotFound | 404 | Account Type Not Found Error | The account type you are trying to access does not exist… |
| 0110 | ErrNoAccountTypesFound | NotFound | 404 | No Account Types Found | No account types were found in the search… |
| 0111 | ErrInvalidAccountRuleType | Validation | 400 | Invalid Account Rule Type | …'account.ruleType'…'alias' or 'account_type'… |
| 0112 | ErrInvalidAccountRuleValue | Validation | 400 | Invalid Account Rule Value | …'account.validIf'… string for 'alias' or array for 'account_type'… |
| 0113 | ErrCorruptedAccountRule | Unprocessable | 422 | Corrupted Account Rule | The account rule data in the operation route is internally inconsistent… |
| 0114 | ErrTransactionRouteNotInformed | Unprocessable | 422 | Transaction Route Not Informed | The transaction route is not informed… |
| 0115 | ErrInvalidTransactionRouteID | Validation | 400 | Invalid Transaction Route ID | …not a valid UUID format… |
| 0116 | ErrAccountingRouteCountMismatch | Unprocessable | 422 | Accounting Route Count Mismatch | …Expected %v source and %v destination operations… %v/%v/%v… |
| 0117 | ErrAccountingRouteNotFound | Unprocessable | 422 | Accounting Route Not Found | The operation route ID '%v' was not found in the transaction route cache for operation '%v'… |
| 0118 | ErrAccountingAliasValidationFailed | Unprocessable | 422 | Accounting Alias Validation Failed | The operation alias '%v' does not match the expected alias '%v'… |
| 0119 | ErrAccountingAccountTypeValidationFailed | Unprocessable | 422 | Accounting Account Type Validation Failed | The account type '%v' does not match any of the expected account types %v… |
| 0120 | ErrInvalidAccountTypeKeyValue | Validation | 400 | Invalid Characters | The field 'keyValue' contains invalid characters… |
| 0121 | ErrInvalidFutureTransactionDate | Validation | 400 | Invalid Future Date Error | The 'transactionDate' cannot be a future date… |
| 0122 | ErrInvalidPendingFutureTransactionDate | Validation | 400 | Invalid Field for Pending Transaction Error | Pending transactions do not support the 'transactionDate' field… |
| 0123 | ErrDuplicatedAliasKeyValue | Conflict | 409 | Duplicated Alias Key Value Error | An account alias with the specified key value already exists… |
| 0124 | ErrAdditionalBalanceNotAllowed | Unprocessable | 422 | Additional Balance Creation Not Allowed | Additional balances are not allowed for external account type. |
| 0125 | ErrInvalidTransactionNonPositiveValue | Unprocessable | 422 | Invalid Transaction Value | Negative or zero transaction values are not allowed. 'send.value' must be > 0. |
| 0126 | ErrDefaultBalanceNotFound | NotFound | 404 | Default Balance Not Found | Default balance must be created first for this account. |
| 0127 | ErrAccountCreationFailed | InternalServer | 500 | Account Creation Failed | …default balance could not be created… |
| 0128 | ErrTransactionBackupCacheFailed | InternalServer | 500 | Transaction Backup Cache Failed | …error while adding the transaction to the backup cache… |
| 0129 | ErrTransactionBackupCacheMarshalFailed | InternalServer | 500 | Transaction Backup Cache Marshal Failed | …error while serializing the transaction for the backup cache… |
| 0131 | ErrInvalidDatetimeFormat | Validation | 400 | Invalid Datetime Format Error | The '%v' parameter is in the incorrect format. Use '%v'… |
| 0132 | ErrMetadataIndexAlreadyExists | Conflict | 409 | Metadata Index Already Exists | A metadata index with the same key already exists… |
| 0133 | ErrMetadataIndexNotFound | NotFound | 404 | Metadata Index Not Found | The specified metadata index does not exist… |
| 0134 | ErrMetadataIndexInvalidKey | Validation | 400 | Invalid Metadata Key Format | …Keys must start with a letter… |
| 0135 | ErrMetadataIndexLimitExceeded | Unprocessable | 422 | Metadata Index Limit Exceeded | The maximum number of metadata indexes has been reached… |
| 0136 | ErrMetadataIndexCreationFailed | InternalServer | 500 | Metadata Index Creation Failed | The metadata index could not be created… |
| 0137 | ErrMetadataIndexDeletionForbidden | Unprocessable | 422 | Metadata Index Deletion Forbidden | System indexes cannot be deleted… |
| 0138 | ErrInvalidEntityName | Validation | 400 | Invalid Entity Name | The provided entity name is not valid. |
| 0139 | ErrTransactionBackupCacheRetrievalFailed | InternalServer | 500 | Transaction Backup Cache Retrieval Failed | …could not be retrieved from the backup cache… |
| 0140 | ErrInvalidTimestamp | Validation | 400 | Invalid Timestamp | The provided timestamp '%v' is invalid. Timestamps cannot be in the future. |
| 0141 | ErrNoBalanceDataAtTimestamp | NotFound | 404 | No Balance Data at Date | No balance data is available at the specified date. |
| 0142 | ErrMissingRequiredQueryParameter | Validation | 400 | Missing Required Query Parameter | The required query parameter '%v' is missing… |
| 0143 | ErrPayloadTooLarge | Validation | 400 | Payload Too Large | The request payload exceeds the maximum allowed size of 64KB. |
| 0144 | ErrJSONNestingDepthExceeded | Validation | 400 | JSON Nesting Depth Exceeded | …maximum allowed nesting depth of 10 levels… |
| 0145 | ErrJSONKeyCountExceeded | Validation | 400 | JSON Key Count Exceeded | …maximum allowed number of keys (100)… |
| 0146 | ErrTenantNotProvisioned | (unmapped) | — | — | (code defined; not in ValidateBusinessError) |
| 0147 | ErrUnknownSettingsField | Validation | 400 | Unknown Settings Field | The settings contain an unknown field: '%v'… |
| 0148 | ErrInvalidSettingsFieldType | Validation | 400 | Invalid Settings Field Type | The settings field '%v' has an invalid type. Expected %v. |
| 0149 | ErrSettingsRootLevelField | Validation | 400 | Settings Field at Root Level | The settings field '%v' must be nested under '%v'… |
| 0150 | ErrRouteNotBidirectional | Unprocessable | 422 | Route Not Bidirectional | …Only routes with operation type 'bidirectional' can be reverted. |
| 0151 | ErrMissingCounterpart | Unprocessable | 422 | Missing Counterpart | Route '%v' requires at least one debit and one credit operation… |
| 0152 | ErrDirectionRouteMismatch | Unprocessable | 422 | Direction Route Mismatch | Operation direction '%v' is not compatible with route operation type '%v' for operation '%v'. |
| 0153 | ErrNoSourceForAction | Validation | 400 | No Source for Action | The action '%v' requires at least one source operation route… |
| 0154 | ErrNoDestinationForAction | Validation | 400 | No Destination for Action | The action '%v' requires at least one destination operation route… |
| 0155 | ErrInvalidRouteAction | Validation | 400 | Invalid Route Action | The action '%v' is not a valid route action… |
| 0157 | ErrNoRoutesForAction | Unprocessable | 422 | No Routes for Action | No routes found for action '%v'… |
| 0158 | ErrTooManyOperationRoutes | Validation | 400 | Too Many Operation Routes | The number of operation routes exceeds the maximum allowed… |
| 0159 | ErrTenantServiceSuspended | (unmapped) | — | — | (code defined; not in ValidateBusinessError) |
| 0160 | ErrTenantNotFound | (unmapped) | — | — | (code defined; not in ValidateBusinessError) |
| 0161 | ErrTenantServiceUnavailable | (unmapped) | — | — | (code defined; not in ValidateBusinessError) |

### 4.2 Accounting-rules block (0162–0166)

| Code | Sentinel | Type | Status | Title | Message |
|------|----------|------|--------|-------|---------|
| 0162 | ErrScenarioNotAllowedForDirection | Unprocessable | 422 | Scenario Not Allowed For Direction | The accounting scenario is not allowed for the specified operation direction. %v |
| 0163 | ErrReserveGroupIncomplete | Unprocessable | 422 | Reserve Group Incomplete | The reserve group (hold, commit, cancel) must be complete. %v |
| 0164 | ErrDirectScenarioRequired | Unprocessable | 422 | Direct Scenario Required | The direct scenario is required when other scenarios are present. %v |
| 0165 | ErrRevertOnlyBidirectional | Unprocessable | 422 | Revert Only Bidirectional | The revert scenario is only allowed for bidirectional operation routes. %v |
| 0166 | ErrAccountingEntryFieldRequired | Unprocessable | 422 | Accounting Entry Field Required | A required field is missing in the accounting entry. %v |

### 4.3 Overdraft / reservation / balance block (0167–0178) — money-path critical

| Code | Sentinel | Type | Status | Title | Message |
|------|----------|------|--------|-------|---------|
| 0167 | ErrOverdraftLimitExceeded | Unprocessable | 422 | Overdraft Limit Exceeded Error | …would exceed the configured overdraft limit for the balance… |
| 0168 | ErrDirectOperationOnInternalBalance | Unprocessable | 422 | Direct Operation On Internal Balance Error | Direct operations on internal-scope balances are not permitted… |
| 0169 | ErrDeletionOfInternalBalance | Unprocessable | 422 | Deletion Of Internal Balance Error | Internal-scope balances cannot be deleted… |
| 0170 | ErrReservedBalanceKey | Unprocessable | 422 | Reserved Balance Key Error | The balance key %v is reserved for system use… |
| 0171 | ErrInvalidBalanceDirection | Validation | 400 | Invalid Balance Direction Error | The balance direction %v is not supported. Allowed: "credit","debit". |
| 0172 | ErrInvalidBalanceSettings | Validation | 400 | Invalid Balance Settings Error | The balance settings payload is invalid… |
| 0173 | ErrOverdraftLimitBelowUsage | Unprocessable | 422 | Overdraft Limit Below Usage Error | The new overdraft limit is below the amount currently used… |
| 0174 | ErrStaleBalanceVersion | Unprocessable | 422 | Stale Balance Version Error | The balance was modified by another transaction between read and write… |
| 0175 | ErrUpdateOfInternalBalance | Unprocessable | 422 | Update Of Internal Balance Error | Internal balances are system-managed and cannot be updated… |
| 0176 | ErrInvalidSettingsFieldValue | Validation | 400 | Invalid Settings Field Value | The settings field '%v' has an invalid value. Allowed values: %v. |
| 0177 | ErrTransactionReservationDenied | Unprocessable | 422 | Transaction Reservation Denied Error | …would exceed a configured usage limit… |
| 0178 | ErrTransactionReservationUnavailable | ServiceUnavailable | 503 | Transaction Reservation Unavailable Error | …usage-limit service is temporarily unavailable and ledger is configured to reject… |

### 4.4 Fee platform block (0179–0233) — money-path (fees)

| Code | Sentinel | Type | Status | Title | Message |
|------|----------|------|--------|-------|---------|
| 0179 | ErrFeeCalculationFieldType | Validation | 400 | Calculation field type invalid | …Values can only be percentage or flat |
| 0180 | ErrPriorityInvalid | Validation | 400 | Invalid fee priority | The priority field in fees is invalid. Field can not be repeated. |
| 0181 | ErrFindAccountOnMidaz | InternalServer | 500 | Account not found on Midaz | Failed to find account '%v' on Midaz… |
| 0182 | ErrMinAmountGreaterThanMaxAmount | Unprocessable | 422 | minimumAmount greater than maximumAmount | minimumAmount value is greater than maximumAmount. |
| 0183 | ErrNothingToUpdate | Validation | 400 | Nothing to Update | No updatable fields were provided… |
| 0184 | ErrDuplicatePackage | Conflict | 409 | Package already exists | A package already exists with the same combination of orgId, ledgerId, segmentId, transactionRoute, min/maxAmount. |
| 0185 | ErrFeeInvalidHeaderParameter | Validation | 400 | Invalid header parameter | One or more header parameters are in an incorrect format %v… |
| 0186 | ErrCalculateFee | InternalServer | 500 | Failed to calculate fee | Failed to calculate the fee for the transaction… |
| 0187 | ErrCalculationRequired | Validation | 400 | Missing calculation model | The calculation model is required for fee %v. |
| 0188 | ErrPriorityOne | Validation | 400 | originalAmount is required when priority is one | For Priority equals to one, referenceAmount must be 'originalAmount' for fee %v. |
| 0189 | ErrAppRuleFlatFeeAndPercentual | Unprocessable | 422 | Failed to apply rule: flatFee or percentual | applicationRule flatFee or percentual must have exactly 1 calculation for Fee %v. |
| 0190 | ErrCalculationTypePercentual | Validation | 400 | Invalid calculation type: percentual | The calculation type percentual must be 'percentage' for Fee %v. |
| 0191 | ErrCalculationTypeFlatFee | Validation | 400 | Invalid calculation type: flatFee | The calculation type flatFee must be 'flat' for Fee %v. |
| 0192 | ErrFeeFieldsRequired | Validation | 400 | Missing required fee fields | All fields of a new Fee must be filled… |
| 0193 | ErrCalculationFieldOfFeeRequired | Validation | 400 | Calculation field is required for fee | Please fill the Calculation object correctly… |
| 0194 | ErrReferenceAmountInvalid | Validation | 400 | referenceAmount is not valid | Field reference amount must be originalAmount or afterFeesAmount. |
| 0195 | ErrAppRuleInvalid | Validation | 400 | Invalid applicationRule | Field application rule must be maxBetweenTypes, flatFee or percentual. |
| 0196 | ErrCalculationTypeInvalid | Validation | 400 | Invalid calculation type | Field calculation type must be percentage or flat. |
| 0197 | ErrMaxAmountLessThanMinAmount | Unprocessable | 422 | maximumAmount less than minimumAmount | maximumAmount value is less than minimumAmount. |
| 0198 | ErrFilterPackage | Validation | 400 | Package filtering error | Failed to filter a single package… |
| 0199 | ErrPackageRange | Conflict | 409 | Package amount range overlap | The maximumAmount and minimumAmount of the new package overlap… |
| 0200 | ErrValidateDistributeTransactionValue | Unprocessable | 422 | Failed to distribute values | Failed to distribute the transaction values… |
| 0201 | ErrAppRuleMaxBetweenTypes | Unprocessable | 422 | Failed to apply rule: maxBetweenTypes | applicationRule maxBetweenTypes must have more than 1 calculation for Fee %v. |
| 0202 | ErrInvalidSegmentID | Validation | 400 | Invalid segmentID | The specified segmentID is not a valid UUID… |
| 0203 | ErrInvalidLedgerID | Validation | 400 | Invalid ledgerID | The specified ledgerID is not a valid UUID… |
| 0204 | ErrConvertToDecimal | InternalServer | 500 | Error to convert values | The value of the field %s is invalid. …dot (.) as decimal separator… |
| 0205 | ErrIsDeductibleFrom | Unprocessable | 422 | originalAmount is required when isDeductibleFrom is true | For isDeductibleFrom `true`, referenceAmount must be 'originalAmount' for fee %v. |
| 0206 | ErrApplicationRule | Validation | 400 | applicationRule invalid value | applicationRule is invalid, Err: %v. |
| 0207 | ErrCalculationValuePercentage | Validation | 400 | calculation value percentage invalid | Calculation value is invalid, it cannot exceed 100%…Fee %v. |
| 0208 | ErrCalculationValueFlatFee | Validation | 400 | calculation value flat invalid | Calculation value…cannot exceed the minimum amount %v…Fee %v. |
| 0209 | ErrAccessMidaz | InternalServer | 500 | Failed to access Midaz | Failed to access Midaz to validate account '%v'… |
| 0210 | ErrDeductibleCalculationValuePercentage | Validation | 400 | deductible value forbidden | Can not update deductible value to true. Calculation value bigger than 100%…Fee %v. |
| 0211 | ErrDeductibleCalculationValueFlatFee | Validation | 400 | deductible value forbidden | Can not update deductible value to true. Calculation value bigger than minimum amount %v for Fee %v. |
| 0212 | ErrInvalidQueryParameterPage | Validation | 400 | Invalid Page | Query parameter page is invalid. The page must be greater than 0. |
| 0213 | ErrBillingPackageNotFound | NotFound | 404 | Billing package not found | No billing package was found for the given ID '%v'. |
| 0214 | ErrInvalidBillingPackageType | Validation | 400 | Invalid billing package type | …Valid types are 'volume' and 'maintenance'. |
| 0215 | ErrMissingVolumeFields | Validation | 400 | Missing volume fields | Volume billing packages require: eventFilter…pricingModel, tiers, assetCode, debit/creditAccountAlias. |
| 0216 | ErrMissingMaintenanceFields | Validation | 400 | Missing maintenance fields | Maintenance billing packages require: feeAmount, assetCode, maintenanceCreditAccount, accountTarget. |
| 0217 | ErrInvalidPricingModel | Validation | 400 | Invalid pricing model | …Valid models are 'tiered' and 'fixed'. |
| 0218 | ErrInvalidPricingTier | Validation | 400 | Invalid pricing tier | (dynamic via formatPricingTierError) |
| 0219 | ErrBillingRouteOverlap | Conflict | 409 | Billing route overlap | A billing package already exists for this org, ledger, and transaction route combination. |
| 0220 | ErrTargetAccountNotFound | NotFound | 404 | Target account not found | The target account '%v' was not found or is inactive in Midaz. |
| 0221 | ErrBillingCalculationFailed | InternalServer | 500 | Billing calculation failed | Failed to calculate billing: %v |
| 0222 | ErrNoActiveBillingPackages | NotFound | 404 | No active billing packages | No active billing packages were found… |
| 0223 | ErrSegmentResolutionFailed | InternalServer | 500 | Segment resolution failed | Failed to resolve accounts for the configured segment… |
| 0224 | ErrInvalidBillingPeriod | Validation | 400 | Invalid billing period | …Use 'YYYY-MM'/'YYYY-Www'/'YYYY-MM-DD'… |
| 0225 | ErrInvalidFreeQuota | Validation | 400 | Invalid free quota | …Must be a non-negative integer. |
| 0226 | ErrInvalidDiscountTier | Validation | 400 | Invalid discount tier | …minQuantity and discountPercentage between 0 and 100. |
| 0227 | ErrInvalidCountMode | Validation | 400 | Invalid count mode | …Valid modes are 'perRoute' and 'perAccount'. |
| 0228 | ErrMidazQueryFailed | ServiceUnavailable | 503 | Service dependency unavailable | A required service is temporarily unavailable… |
| 0229 | ErrInvalidAccountTarget | Validation | 400 | Invalid account target | …Exactly one of segmentId, portfolioId, or aliases must be provided. |
| 0230 | ErrInvalidFeeAmount | Validation | 400 | Invalid fee amount | …must be a positive value greater than zero. |
| 0231 | ErrMissingSegmentContext | FailedPrecondition | 500 | Segment context unavailable | Segment-based waivers are configured but the resolution service is not available… |
| 0232 | ErrMidazRouteNotFound | NotFound | 404 | Midaz service route not found | The Midaz service endpoint returned 404 (route not found)… |
| 0233 | ErrDeductibleFeeExceedsAmount | Unprocessable | 422 | Deductible fee exceeds the amount it deducts from | A deductible fee cannot be applied because it meets or exceeds the amount it deducts from… |

### 4.5 Tracer platform block (0328–0491)

| Code | Sentinel | Type | Status | Title | Message |
|------|----------|------|--------|-------|---------|
| 0328 | ErrRuleCalculationFieldType | Validation | 400 | Calculation Field Type | Invalid calculation field type. |
| 0329 | ErrParentIDNotFound | NotFound | 404 | Parent IDNot Found | Parent ID not found. |
| 0330 | ErrContextCancelled | ServiceUnavailable | 503 | Context Cancelled | Context cancelled / service unavailable. |
| 0331 | ErrPaginationLimitInvalid | Validation | 400 | Pagination Limit Invalid | Pagination limit must be positive. |
| 0332 | ErrInvalidSortColumn | Validation | 400 | Invalid Sort Column | Sort column not in allowed list. |
| 0333 | ErrInvalidCursor | Validation | 400 | Invalid Cursor | Invalid or corrupted pagination cursor. |
| 0334 | ErrCursorWithSortParams | Validation | 400 | Cursor With Sort Params | Cursor and sort parameters are mutually exclusive. |
| 0335 | ErrMetadataEntriesExceeded | Validation | 400 | Metadata Entries Exceeded | Metadata entries exceed maximum of 50. |
| 0336 | ErrMetadataKeyInvalidChars | Validation | 400 | Metadata Key Invalid Chars | Metadata key contains invalid characters. |
| 0337 | ErrInvalidDecision | Validation | 400 | Invalid Decision | Invalid decision value. |
| 0338 | ErrReasonRequired | Validation | 400 | Reason Required | Reason is required. |
| 0339 | ErrInvalidDefaultDecision | Validation | 400 | Invalid Default Decision | Invalid default decision value. |
| 0340 | ErrExpressionSyntax | Validation | 400 | Expression Syntax | Invalid CEL syntax. |
| 0341 | ErrExpressionType | Validation | 400 | Expression Type | Expression must return boolean. |
| 0342 | ErrExpressionCostExceeded | Unprocessable | 422 | Expression Cost Exceeded | Cost limit exceeded (cost computed and above threshold). |
| 0343 | ErrExpressionEvaluation | InternalServer | 500 | Expression Evaluation | Runtime evaluation error. |
| 0344 | ErrExpressionProgram | Validation | 400 | Expression Program | Program creation failed (compilation phase). |
| 0345 | ErrExpressionCostEstimation | InternalServer | 500 | Expression Cost Estimation | Failed to estimate expression cost. |
| 0346 | ErrAmountExceedsPrecision | Unprocessable | 422 | Amount Exceeds Precision | Amount exceeds safe precision for CEL float64 evaluation (max ±2^53). |
| 0347 | ErrRuleNotFound | NotFound | 404 | Rule Not Found | Rule not found by ID. |
| 0348 | ErrRuleNameAlreadyExists | Conflict | 409 | Rule Name Already Exists | Rule name must be unique. |
| 0349 | ErrRuleInvalidStatus | Unprocessable | 422 | Rule Invalid Status | Invalid rule status transition. |
| 0350 | ErrRuleEvaluationFailed | InternalServer | 500 | Rule Evaluation Failed | Rule evaluation failed. |
| 0351 | ErrExpressionNotModifiable | Unprocessable | 422 | Expression Not Modifiable | Expression cannot be modified for non-DRAFT rules. |
| 0352 | ErrRuleNilInput | Validation | 400 | Rule Nil Input | Rule input cannot be nil. |
| 0353 | ErrRuleNameRequired | Validation | 400 | Rule Name Required | Rule name is required. |
| 0354 | ErrRuleNameTooLong | Validation | 400 | Rule Name Too Long | Rule name exceeds max length (255). |
| 0355 | ErrRuleExpressionRequired | Validation | 400 | Rule Expression Required | Rule expression is required. |
| 0356 | ErrRuleExpressionTooLong | Validation | 400 | Rule Expression Too Long | Rule expression exceeds max length (5000). |
| 0357 | ErrRuleInvalidAction | Validation | 400 | Rule Invalid Action | Action must be one of [ALLOW, DENY, REVIEW]. |
| 0358 | ErrRuleInvalidScope | Validation | 400 | Rule Invalid Scope | Scope must have at least one field set. |
| 0359 | ErrRuleDescriptionTooLong | Validation | 400 | Rule Description Too Long | Rule description exceeds max length (1000). |
| 0360 | ErrRuleScopesTooMany | Validation | 400 | Rule Scopes Too Many | Rule scopes exceed maximum (100). |
| 0361 | ErrRuleInvalidTransition | Unprocessable | 422 | Rule Invalid Transition | Status transition not allowed. |
| 0362 | ErrLimitNotFound | NotFound | 404 | Limit Not Found | Limit not found by ID. |
| 0363 | ErrLimitInvalidStatusChange | Unprocessable | 422 | Limit Invalid Status Change | Invalid limit status transition. |
| 0364 | ErrLimitInvalidType | Validation | 400 | Limit Invalid Type | Invalid limit type. |
| 0365 | ErrLimitInvalidMaxAmount | Validation | 400 | Limit Invalid Max Amount | MaxAmount must be positive. |
| 0366 | ErrLimitInvalidCurrency | Validation | 400 | Limit Invalid Currency | Currency must be valid ISO 4217. |
| 0367 | ErrLimitInvalidScope | Validation | 400 | Limit Invalid Scope | Scope validation failed. |
| 0368 | ErrLimitNameRequired | Validation | 400 | Limit Name Required | Limit name is required. |
| 0369 | ErrLimitNameTooLong | Validation | 400 | Limit Name Too Long | Limit name exceeds max length. |
| 0370 | ErrLimitAlreadyDeleted | Unprocessable | 422 | Limit Already Deleted | Limit is already in DELETED state. |
| 0371 | ErrLimitNameInvalidChars | Validation | 400 | Limit Name Invalid Chars | Limit name contains invalid characters. |
| 0372 | ErrLimitDescriptionInvalidChars | Validation | 400 | Limit Description Invalid Chars | Limit description contains invalid characters. |
| 0373 | ErrLimitInvalidID | Validation | 400 | Limit Invalid ID | Limit ID is invalid or nil. |
| 0374 | ErrLimitDescriptionTooLong | Validation | 400 | Limit Description Too Long | Limit description exceeds max length. |
| 0375 | ErrLimitInvalidStatusFilter | Validation | 400 | Limit Invalid Status Filter | Invalid status filter value. |
| 0376 | ErrLimitInvalidTypeFilter | Validation | 400 | Limit Invalid Type Filter | Invalid limitType filter value. |
| 0377 | ErrLimitDeletedAtInvariant | InternalServer | 500 | Limit Deleted At Invariant | DeletedAt must be set iff status is DELETED. |
| 0378 | ErrLimitCheckFailed | InternalServer | 500 | Limit Check Failed | Limit check failed. |
| 0379 | ErrLimitNilInput | Validation | 400 | Limit Nil Input | Limit input cannot be nil. |
| 0380 | ErrLimitImmutableField | Unprocessable | 422 | Limit Immutable Field | Cannot modify immutable field (limitType, currency). |
| 0381 | ErrAuditEventNotFound | NotFound | 404 | Audit Event Not Found | Audit event not found. |
| 0382 | ErrInvalidAuditEventFilters | Validation | 400 | Invalid Audit Event Filters | Invalid audit event filter parameters. |
| 0383 | ErrAuditEventInvalidType | Validation | 400 | Audit Event Invalid Type | Invalid audit event type. |
| 0384 | ErrAuditEventInvalidAction | Validation | 400 | Audit Event Invalid Action | Invalid audit action. |
| 0385 | ErrAuditEventInvalidResult | Validation | 400 | Audit Event Invalid Result | Invalid audit result. |
| 0386 | ErrAuditEventResourceIDRequired | Validation | 400 | Audit Event Resource IDRequired | Resource ID is required. |
| 0387 | ErrAuditEventInvalidResourceType | Validation | 400 | Audit Event Invalid Resource Type | Invalid resource type. |
| 0388 | ErrAuditEventActorIDRequired | Validation | 400 | Audit Event Actor IDRequired | Actor ID is required. |
| 0389 | ErrAuditEventActorTypeInvalid | Validation | 400 | Audit Event Actor Type Invalid | Actor type must be 'user' or 'system'. |
| 0390 | ErrUsageCounterOverflow | InternalServer | 500 | Usage Counter Overflow | Usage counter would overflow. |
| 0391 | ErrUsageCounterLimitIDRequired | Validation | 400 | Usage Counter Limit IDRequired | Usage counter limitID is required. |
| 0392 | ErrUsageCounterScopeKeyRequired | Validation | 400 | Usage Counter Scope Key Required | Usage counter scopeKey is required. |
| 0393 | ErrUsageCounterPeriodKeyRequired | Validation | 400 | Usage Counter Period Key Required | Usage counter periodKey is required. |
| 0394 | ErrUsageCounterCurrentUsageNegative | Validation | 400 | Usage Counter Current Usage Negative | Usage counter currentUsage must be non-negative. |
| 0395 | ErrUsageCounterIncrementNonNegative | Validation | 400 | Usage Counter Increment Non Negative | Increment amount must be non-negative. |
| 0396 | ErrUsageCounterNotFound | NotFound | 404 | Usage Counter Not Found | Usage counter not found. |
| 0397 | ErrUsageCounterExceedsLimit | Unprocessable | 422 | Usage Counter Exceeds Limit | Usage counter increment would exceed limit maximum. |
| 0398 | ErrUsageCounterDecrementNonNegative | Validation | 400 | Usage Counter Decrement Non Negative | Decrement amount must be non-negative. |
| 0399 | ErrCheckLimitsInvalidAmount | Validation | 400 | Check Limits Invalid Amount | Check limits amount must be positive. |
| 0400 | ErrCheckLimitsInvalidCurrency | Validation | 400 | Check Limits Invalid Currency | Check limits currency must be valid ISO 4217. |
| 0401 | ErrCheckLimitsUnknownLimitType | Validation | 400 | Check Limits Unknown Limit Type | Unknown limit type for period key calculation. |
| 0402 | ErrCheckLimitsInvalidTimestamp | Validation | 400 | Check Limits Invalid Timestamp | Check limits timestamp must not be zero. |
| 0403 | ErrCheckLimitsNilInput | Validation | 400 | Check Limits Nil Input | Check limits input cannot be nil. |
| 0404 | ErrCheckLimitsInvalidAccountID | Validation | 400 | Check Limits Invalid Account ID | Check limits accountId is required. |
| 0405 | ErrCheckLimitsInvalidTransactionType | Validation | 400 | Check Limits Invalid Transaction Type | Check limits transactionType must be valid. |
| 0406 | ErrCheckLimitsInvalidSubType | Validation | 400 | Check Limits Invalid Sub Type | Check limits subType exceeds maximum length. |
| 0407 | ErrCheckLimitsInvalidSegmentID | Validation | 400 | Check Limits Invalid Segment ID | Check limits segmentId must not be zero UUID. |
| 0408 | ErrCheckLimitsInvalidPortfolioID | Validation | 400 | Check Limits Invalid Portfolio ID | Check limits portfolioId must not be zero UUID. |
| 0409 | ErrCheckLimitsInvalidMerchantID | Validation | 400 | Check Limits Invalid Merchant ID | Check limits merchantId must not be zero UUID. |
| 0410 | ErrLimitCheckerNilLimitRepo | InternalServer | 500 | Limit Checker Nil Limit Repo | Limit checker: limit repository cannot be nil. |
| 0411 | ErrLimitCheckerNilUsageCounterRepo | InternalServer | 500 | Limit Checker Nil Usage Counter Repo | Limit checker: usage counter repository cannot be nil. |
| 0412 | ErrLimitCheckerNilClock | InternalServer | 500 | Limit Checker Nil Clock | Limit checker: clock cannot be nil. |
| 0413 | ErrValidationRequestIDRequired | Validation | 400 | Validation Request IDRequired | RequestId is required. |
| 0414 | ErrValidationInvalidTransactionType | Validation | 400 | Validation Invalid Transaction Type | Invalid transactionType. |
| 0415 | ErrValidationAmountNonPositive | Validation | 400 | Validation Amount Non Positive | Amount must be positive. |
| 0416 | ErrValidationCurrencyRequired | Validation | 400 | Validation Currency Required | Currency is required. |
| 0417 | ErrValidationInvalidCurrency | Validation | 400 | Validation Invalid Currency | Currency must be valid ISO 4217. |
| 0418 | ErrValidationTimestampRequired | Validation | 400 | Validation Timestamp Required | Timestamp is required. |
| 0419 | ErrValidationTimestampFuture | Validation | 400 | Validation Timestamp Future | Timestamp cannot be in the future. |
| 0420 | ErrValidationAccountRequired | Validation | 400 | Validation Account Required | Account is required. |
| 0421 | ErrValidationTimestampPast | Validation | 400 | Validation Timestamp Past | Timestamp is too far in the past. |
| 0422 | ErrValidationTimeout | ServiceUnavailable | 503 | Validation Timeout | Validation timeout. |
| 0423 | ErrValidationSegmentIDRequired | Validation | 400 | Validation Segment IDRequired | SegmentId is required when segment is provided. |
| 0424 | ErrValidationPortfolioIDRequired | Validation | 400 | Validation Portfolio IDRequired | PortfolioId is required when portfolio is provided. |
| 0425 | ErrValidationSubTypeTooLong | Validation | 400 | Validation Sub Type Too Long | SubType exceeds maximum length of 50 characters. |
| 0426 | ErrValidationInvalidAccountType | Validation | 400 | Validation Invalid Account Type | Account.type must be checking, savings, or credit. |
| 0427 | ErrValidationInvalidAccountStatus | Validation | 400 | Validation Invalid Account Status | Account.status must be active, suspended, or closed. |
| 0428 | ErrValidationInvalidMerchantCategory | Validation | 400 | Validation Invalid Merchant Category | Merchant.category must be 4-digit MCC code. |
| 0429 | ErrValidationInvalidMerchantCountry | Validation | 400 | Validation Invalid Merchant Country | Merchant.country must be ISO 3166-1 alpha-2. |
| 0430 | ErrValidationMerchantIDRequired | Validation | 400 | Validation Merchant IDRequired | Merchant.id is required when merchant is provided. |
| 0431 | ErrInvalidTransactionValidationFilters | Validation | 400 | Invalid Transaction Validation Filters | Invalid transaction validation filter parameters. |
| 0432 | ErrTransactionValidationNotFound | NotFound | 404 | Transaction Validation Not Found | Transaction validation record not found. |
| 0433 | ErrListValidationsTimeout | ServiceUnavailable | 503 | List Validations Timeout | List validations query timeout (deadline exceeded). |
| 0434 | ErrTransactionValidationIDRequired | Validation | 400 | Transaction Validation IDRequired | Validation ID is required. |
| 0435 | ErrTransactionValidationCreatedAtRequired | Validation | 400 | Transaction Validation Created At Required | CreatedAt is required. |
| 0436 | ErrRuleCacheWarmUpFailed | FailedPrecondition | 500 | Rule Cache Warm Up Failed | Rule cache warm-up failed. |
| 0437 | ErrRuleCacheNotReady | ServiceUnavailable | 503 | Rule Cache Not Ready | Rule cache is not ready. |
| 0438 | ErrLimitTimeWindowMismatch | Validation | 400 | Limit Time Window Mismatch | ActiveTimeStart and activeTimeEnd must both be set or both be nil. |
| 0439 | ErrLimitTimeWindowZeroWidth | Validation | 400 | Limit Time Window Zero Width | ActiveTimeStart cannot equal activeTimeEnd. |
| 0440 | ErrTimeOfDayInvalidFormat | Validation | 400 | Time Of Day Invalid Format | Invalid time of day format, expected HH:MM. |
| 0441 | ErrRuleNameAlreadyExistsInCtx | Conflict | 409 | Rule Name Already Exists In Ctx | Rule name already exists in this context. |
| 0442 | ErrLimitNameAlreadyExists | Conflict | 409 | Limit Name Already Exists | Limit name already exists. |
| 0443 | ErrLimitCustomDatesNotAllowed | Validation | 400 | Limit Custom Dates Not Allowed | CustomStartDate/customEndDate only allowed for CUSTOM limitType. |
| 0444 | ErrLimitUnknownType | Validation | 400 | Limit Unknown Type | Unknown limit type. |
| 0445 | ErrLimitCustomPeriodTooLong | Unprocessable | 422 | Limit Custom Period Too Long | Custom period cannot exceed 5 years. |
| 0446 | ErrLimitCustomPeriodExpired | Unprocessable | 422 | Limit Custom Period Expired | Custom period end date must be in the future. |
| 0447 | ErrLimitInvalidCustomStartFormat | Validation | 400 | Limit Invalid Custom Start Format | Invalid customStartDate format, expected RFC3339. |
| 0448 | ErrLimitInvalidCustomEndFormat | Validation | 400 | Limit Invalid Custom End Format | Invalid customEndDate format, expected RFC3339. |
| 0449 | ErrLimitCustomDatesRequired | Validation | 400 | Limit Custom Dates Required | CustomStartDate and customEndDate required for CUSTOM limitType. |
| 0450 | ErrLimitCustomDatesOrder | Validation | 400 | Limit Custom Dates Order | CustomStartDate must be before customEndDate. |
| 0451 | ErrMTConfigRequired | InternalServer | 500 | MTConfig Required | Multi-tenant config: cfg is required. |
| 0452 | ErrMTLoggerRequired | InternalServer | 500 | MTLogger Required | Multi-tenant config: logger is required. |
| 0453 | ErrMTURLRequired | FailedPrecondition | 500 | MTURLRequired | MULTI_TENANT_URL must be set when MULTI_TENANT_ENABLED=true. |
| 0454 | ErrMTURLInvalid | FailedPrecondition | 500 | MTURLInvalid | MULTI_TENANT_URL must be a valid absolute URL… |
| 0455 | ErrMTServiceAPIKeyRequired | FailedPrecondition | 500 | MTService APIKey Required | MULTI_TENANT_SERVICE_API_KEY must be set… |
| 0456 | ErrMTRedisHostRequired | FailedPrecondition | 500 | MTRedis Host Required | MULTI_TENANT_REDIS_HOST must be set… |
| 0457 | ErrMTPluginAuthRequired | FailedPrecondition | 500 | MTPlugin Auth Required | MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true. |
| 0458 | ErrMTAPIKeyOnlyValidationConfl | FailedPrecondition | 500 | MTAPIKey Only Validation Confl | MULTI_TENANT_ENABLED=true incompatible with API_KEY_ENABLED_ONLY_VALIDATION=true. |
| 0459 | ErrReadyzPgConnectionNotEstablished | ServiceUnavailable | 503 | Readyz Pg Connection Not Established | Postgres readyz: connection not established. |
| 0460 | ErrReadyzPgConnectionFailed | ServiceUnavailable | 503 | Readyz Pg Connection Failed | Postgres readyz: connection failed. |
| 0461 | ErrReadyzPgPingFailed | ServiceUnavailable | 503 | Readyz Pg Ping Failed | Postgres readyz: ping failed. |
| 0462 | ErrReadyzDependenciesUnhealthy | ServiceUnavailable | 503 | Readyz Dependencies Unhealthy | /readyz aggregate: one or more dependencies unhealthy. |
| 0463 | ErrReadyzCacheNotReady | ServiceUnavailable | 503 | Readyz Cache Not Ready | Rule_cache readyz: cache not ready. |
| 0464 | ErrReadyzCacheStale | ServiceUnavailable | 503 | Readyz Cache Stale | Rule_cache readyz: cache data stale. |
| 0465 | ErrSupervisorShuttingDown | ServiceUnavailable | 503 | Supervisor Shutting Down | Worker supervisor: shutting down, refusing to spawn new tenant workers. |
| 0466 | ErrTenantCapReached | ServiceUnavailable | 503 | Tenant Cap Reached | Tenant worker cap reached; client should retry after backoff. |
| 0467 | ErrSupervisorNilRuleCache | InternalServer | 500 | Supervisor Nil Rule Cache | Worker supervisor: rule cache is required. |
| 0468 | ErrSupervisorNilSyncRepo | InternalServer | 500 | Supervisor Nil Sync Repo | Worker supervisor: sync repo is required. |
| 0469 | ErrSupervisorNilUsageRepo | InternalServer | 500 | Supervisor Nil Usage Repo | Worker supervisor: usage repo is required when cleanup workers are enabled. |
| 0470 | ErrSupervisorNilCompiler | InternalServer | 500 | Supervisor Nil Compiler | Worker supervisor: compiler is required. |
| 0471 | ErrSupervisorNilLogger | InternalServer | 500 | Supervisor Nil Logger | Worker supervisor: logger is required. |
| 0472 | ErrSupervisorNilReaperRepo | InternalServer | 500 | Supervisor Nil Reaper Repo | Worker supervisor: reservation reaper repo is required when reaper workers are enabled. |
| 0473 | ErrSupervisorNilReaperAuditor | InternalServer | 500 | Supervisor Nil Reaper Auditor | Worker supervisor: reservation reaper auditor is required when reaper workers are enabled. |
| 0474 | ErrUnauthorizedMissingSub | Unauthorized | 401 | Unauthorized Missing Sub | JWT lacks required 'sub' claim — identity cannot be attributed. |
| 0475 | ErrReservationLimitIDRequired | Validation | 400 | Reservation Limit IDRequired | Reservation: limitId is required. |
| 0476 | ErrReservationTransactionIDReq | Validation | 400 | Reservation Transaction IDReq | Reservation: transactionId is required. |
| 0477 | ErrReservationScopeKeyRequired | Validation | 400 | Reservation Scope Key Required | Reservation: scopeKey is required. |
| 0478 | ErrReservationPeriodKeyRequired | Validation | 400 | Reservation Period Key Required | Reservation: periodKey is required. |
| 0479 | ErrReservationAmountInvalid | Validation | 400 | Reservation Amount Invalid | Reservation: amount must be non-negative. |
| 0480 | ErrReservationInvalidStatus | Validation | 400 | Reservation Invalid Status | Reservation: status must be one of RESERVED, CONFIRMED, RELEASED, EXPIRED. |
| 0481 | ErrReservationExpiresAtRequired | Validation | 400 | Reservation Expires At Required | Reservation: reservationExpiresAt is required. |
| 0482 | ErrReservationNotFound | NotFound | 404 | Reservation Not Found | Reservation: reservation not found. |
| 0483 | ErrReservationAlreadyTerminal | Unprocessable | 422 | Reservation Already Terminal | Reservation: reservation is already in a terminal state. |
| 0484 | ErrRouteNotFound | NotFound | 404 | Route Not Found | The requested route does not exist. Please verify the HTTP method and path… |
| 0485 | ErrMethodNotAllowed | Validation | 400 | Method Not Allowed | The HTTP method is not allowed for the requested route… |
| 0486 | ErrPendingTransactionLocked | Conflict | 409 | Transaction Locked | This transaction is currently being processed by another request. Please retry shortly. |
| 0487 | ErrReservationTenantRequired | Validation | 400 | Reservation Tenant Required | Reservation: tenant id is required on the multi-tenant reservation surface. |
| 0488 | ErrInstrumentLedgerReferenceNotFound | Unprocessable | 422 | Instrument Ledger Reference Not Found | The ledger referenced by this instrument does not exist in this organization… |
| 0489 | ErrInstrumentAccountReferenceNotFound | Unprocessable | 422 | Instrument Account Reference Not Found | The account referenced by this instrument does not exist in the referenced ledger… |
| 0490 | ErrSkipNotPermitted | Unprocessable | 422 | Skip Not Permitted | The %v skip requested for this operation is not permitted on this ledger… |
| 0491 | ErrHolderRequired | Unprocessable | 422 | Holder Required | This ledger requires every account to name an existing holder… |

### 4.6 CRM block (`CRM-00xx`)

| Code | Sentinel | Type | Status | Title | Message |
|------|----------|------|--------|-------|---------|
| CRM-0006 | ErrHolderNotFound | NotFound | 404 | Holder ID Not Found | The provided holder ID does not exist… |
| CRM-0008 | ErrInstrumentNotFound | NotFound | 404 | Instrument ID Not Found | The provided instrument ID does not exist… |
| CRM-0010 | ErrDocumentAssociationError | Conflict | 409 | Document Association Error | A document can only be associated with one holder. |
| CRM-0013 | ErrAccountAlreadyAssociated | Conflict | 409 | Account Already Associated | An accountId from ledger can only be associated with a single related account on CRM. |
| CRM-0017 | ErrHolderHasInstruments | Unprocessable | 422 | Unable to Delete Holder | The holder cannot be deleted because it has one or more associated aliases. |
| CRM-0019 | ErrMetadataQueryInvalidFormat | (unmapped) | — | — | (defined; not in ValidateBusinessError) |
| CRM-0020 | ErrMetadataQueryInvalidKey | (unmapped) | — | — | (defined; not in ValidateBusinessError) |
| CRM-0021 | ErrMetadataQueryContainsOperator | (unmapped) | — | — | (defined; not in ValidateBusinessError) |
| CRM-0022 | ErrInvalidHeaderValue | (unmapped) | — | — | (defined; not in ValidateBusinessError) |
| CRM-0023 | ErrInstrumentClosingDateBeforeCreation | Validation | 400 | Alias Closing Date Before Creation Date | The alias closing date cannot be before the creation date… |
| CRM-0024 | ErrRelatedPartyNotFound | NotFound | 404 | Related Party Not Found | The specified related party does not exist… |
| CRM-0025 | ErrInvalidRelatedPartyRole | Validation | 400 | Invalid Related Party Role | …PRIMARY_HOLDER, LEGAL_REPRESENTATIVE, or RESPONSIBLE_PARTY. |
| CRM-0026 | ErrRelatedPartyDocumentRequired | Validation | 400 | Related Party Document Required | The related party document is required… |
| CRM-0027 | ErrRelatedPartyNameRequired | Validation | 400 | Related Party Name Required | The related party name is required… |
| CRM-0028 | ErrRelatedPartyStartDateRequired | Validation | 400 | Related Party Start Date Required | The related party start date is required… |
| CRM-0029 | ErrRelatedPartyEndDateInvalid | Validation | 400 | Related Party End Date Invalid | The related party end date must be after the start date… |
| CRM-0030 | ErrHolderHasAccounts | Unprocessable | 422 | Unable to Delete Holder | The holder cannot be deleted because it owns one or more active accounts. |
| CRM-0031 | ErrKeysetNotFound | (unmapped) | — | — | (defined; not in ValidateBusinessError) |
| CRM-0032 | ErrKeysetAlreadyExists | (unmapped) | — | — | (defined; not in ValidateBusinessError) |
| CRM-0033 | ErrKeysetRevisionConflict | (unmapped) | — | — | (defined; not in ValidateBusinessError) |
| CRM-0034 | ErrRegistryNotFound | NotFound | 404 | Organization Not Provisioned | The organization has not been provisioned for envelope encryption… |
| CRM-0035 | ErrRegistryAlreadyExists | Conflict | 409 | Organization Already Provisioned | The organization has already been provisioned for envelope encryption. |
| CRM-0036 | ErrRegistryRevisionConflict | (unmapped) | — | — | (defined; not in ValidateBusinessError) |
| CRM-0037 | ErrOrganizationEncryptionFailed | InternalServer | 500 | Encryption Operation Failed | The encryption operation failed… |
| CRM-0038 | ErrProvisioningFailed | InternalServer | 500 | Provisioning Failed | The provisioning operation failed… |
| CRM-0039 | ErrAuditEventRequired | Validation | 400 | Missing Fields in Request | Your request is missing one or more required fields… |
| CRM-0040 | ErrAuditWriteFailed | (unmapped) | — | — | (defined; not in ValidateBusinessError) |
| CRM-0041 | ErrReservedTenantID | Unprocessable | 422 | Reserved Tenant ID | The tenant id "default" is reserved for internal single-tenant use… |

---

## 5. Notes for the consolidation

1. **Status is type-derived, not code-derived.** The only status-routing logic is `WithError`'s type switch. There is no per-code status table — status comes entirely from which Go error type `ValidateBusinessError` assigns. To change a code's status you change its type.
2. **`FailedPreconditionError` → 500, not 412.** Every JWK/enforcer/multi-tenant-config/cache-warmup precondition failure (0044, 0045, 0231, 0436, 0453-0458) surfaces as 500. If the consolidation expects 412 Precondition Failed, this is a mismatch to flag.
3. **`ResponseError.Code` is an HTTP status, not an `00xx`.** Only two producers (`ValidateUnmarshallingError` → 0094, and callers passing raw status). Do not treat `ResponseError` uniformly with the other typed errors.
4. **`fields` map only ships on 400 validation-with-fields paths** (`ValidationKnownFieldsError`/`ValidationUnknownFieldsError`). Plain `ValidationError` (0004, 0171, …) is repacked with `fields:nil` — client sees no field detail.
5. **18 defined-but-unmapped sentinels** (see §3) are still wire-valid code strings but do not route through the central mapper — they are surfaced by `ValidateInternalError`/`ValidateBadRequestFieldsError`/`ValidateUnmarshallingError` or other handler-local paths. Money-path relevant among these: `ErrNoBalancesFound` (0092).
6. **Numeric reserved band 0234-0327** (94 codes) is an intentional gap between the fee and tracer blocks; do not reuse without checking the migration plan `docs/plans/2026-06-07-error-code-migration.md`.
7. Fee money-path codes named in the task (0188 `ErrPriorityOne`, 0205 `ErrIsDeductibleFrom`) confirmed present; both are 400/422 fee-validation codes, not settlement codes.
