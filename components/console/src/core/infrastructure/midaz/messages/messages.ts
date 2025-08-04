import { defineMessages } from '@/lib/intl'

export const apiErrorMessages = defineMessages({
  '0001': {
    id: 'errors.midaz.duplicateLedgerName',
    defaultMessage:
      'A ledger with this name already exists in the organization. Please rename the ledger or choose a different organization to attach it to.'
  },
  '0002': {
    id: 'errors.midaz.ledgerNameConflict',
    defaultMessage:
      'A ledger with this name already exists in the organization. Please rename the ledger or choose a different organization to attach it to.'
  },
  '0003': {
    id: 'errors.midaz.assetNameOrCodeDuplicate',
    defaultMessage:
      'An asset with the same name or code already exists in your ledger. Please modify the name or code of your new asset.'
  },
  '0004': {
    id: 'errors.midaz.codeUpperCaseRequirement',
    defaultMessage:
      'The code must be in uppercase. Please ensure that the code is in uppercase format and try again.'
  },
  '0005': {
    id: 'errors.midaz.currencyCodeStandardCompliance',
    defaultMessage:
      'Currency-type assets must comply with the ISO-4217 standard. Please use a currency code that conforms to ISO-4217 guidelines.'
  },
  '0006': {
    id: 'errors.midaz.unmodifiableFieldError',
    defaultMessage:
      'Your request includes a field that cannot be modified. Please review your request and try again, removing any uneditable fields. Please refer to the documentation for guidance.'
  },
  '0007': {
    id: 'errors.midaz.entityNotFound',
    defaultMessage:
      'No entity was found for the given ID. Please make sure to use the correct ID for the entity you are trying to manage.'
  },
  '0008': {
    id: 'errors.midaz.actionNotPermitted',
    defaultMessage:
      'The action you are attempting is not allowed in the current environment. Please refer to the documentation for guidance.'
  },
  '0009': {
    id: 'errors.midaz.missingFieldsInRequest',
    defaultMessage:
      'Your request is missing one or more required fields. Please refer to the documentation to ensure all necessary fields are included in your request.'
  },
  '0010': {
    id: 'errors.midaz.accountTypeImmutable',
    defaultMessage:
      'The account type specified cannot be modified. Please ensure the correct account type is being used and try again.'
  },
  '0011': {
    id: 'errors.midaz.inactiveAccountTypeError',
    defaultMessage:
      'The account type specified cannot be set to INACTIVE. Please ensure the correct account type is being used and try again.'
  },
  '0012': {
    id: 'errors.midaz.accountBalanceDeletionError',
    defaultMessage:
      'An account or sub-account cannot be deleted if it has a remaining balance. Please ensure all remaining balances are transferred to another account before attempting to delete.'
  },
  '0013': {
    id: 'errors.midaz.resourceAlreadyDeleted',
    defaultMessage:
      'The resource you are trying to delete has already been deleted. Ensure you are using the correct ID and try again.'
  },
  '0014': {
    id: 'errors.midaz.segmentIdInactive',
    defaultMessage:
      'The Segment ID you are attempting to use is inactive. Please use another Segment ID and try again.'
  },
  '0015': {
    id: 'errors.midaz.duplicateSegmentNameError',
    defaultMessage:
      'A segment with this name already exists for this ledger. Please try again with a different ledger or name.'
  },
  '0016': {
    id: 'errors.midaz.balanceRemainingDeletionError',
    defaultMessage:
      'The asset cannot be deleted because there is a remaining balance. Please ensure all balances are cleared before attempting to delete again.'
  },
  '0017': {
    id: 'errors.midaz.invalidScriptFormatError',
    defaultMessage:
      'The script provided in your request is invalid or in an unsupported format. Please verify the script format and try again.'
  },
  '0018': {
    id: 'errors.midaz.insufficientFundsError',
    defaultMessage:
      'The transaction could not be completed due to insufficient funds in the account. Please add sufficient funds to your account and try again.'
  },
  '0019': {
    id: 'errors.midaz.accountIneligibilityError',
    defaultMessage:
      'One or more accounts listed in the transaction are not eligible to participate. Please review the account statuses and try again.'
  },
  '0020': {
    id: 'errors.midaz.aliasUnavailabilityError',
    defaultMessage:
      'The alias is already in use. Please choose a different alias and try again.'
  },
  '0021': {
    id: 'errors.midaz.parentTransactionIdNotFound',
    defaultMessage:
      'The parentTransactionId does not correspond to any existing transaction. Please review the ID and try again.'
  },
  '0022': {
    id: 'errors.midaz.immutableFieldError',
    defaultMessage:
      'The field cannot be modified. Please remove this field from your request and try again.'
  },
  '0024': {
    id: 'errors.midaz.accountStatusTransactionRestriction',
    defaultMessage:
      'The current statuses of the source and/or destination accounts do not permit transactions. Change the account status(es) and try again.'
  },
  '0025': {
    id: 'errors.midaz.insufficientAccountBalanceError',
    defaultMessage:
      'The account does not have sufficient balance. Please try again with an amount that is less than or equal to the available balance.'
  },
  '0026': {
    id: 'errors.midaz.transactionMethodRestriction',
    defaultMessage:
      'Transactions involving this asset code are not permitted for the specified source and/or destination. Please try again using accounts that allow transactions with the asset code.'
  },
  '0027': {
    id: 'errors.midaz.duplicateTransactionTemplateCodeError',
    defaultMessage:
      'A transaction template with the this code already exists for your ledger. Please use a different code and try again.'
  },
  '0028': {
    id: 'errors.midaz.duplicateAssetPairError',
    defaultMessage:
      'A pair for the assets already exists. Please update the existing entry instead of creating a new one.'
  },
  '0029': {
    id: 'errors.midaz.invalidParentAccountId',
    defaultMessage:
      'The specified parent account ID does not exist. Please verify the ID is correct and try your request again.'
  },
  '0030': {
    id: 'errors.midaz.mismatchedAssetCode',
    defaultMessage:
      'The parent account ID you provided is associated with a different asset code than the one specified in your request. Please make sure the asset code matches that of the parent account, or use a different parent account ID and try again.'
  },
  '0031': {
    id: 'errors.midaz.chartTypeNotFound',
    defaultMessage:
      'The chart type does not exist. Please provide a valid chart type and refer to the documentation if you have any questions.'
  },
  '0032': {
    id: 'errors.midaz.invalidCountryCode',
    defaultMessage:
      "The provided country code in the 'address.country' field does not conform to the ISO-3166 alpha-2 standard. Please provide a valid alpha-2 country code."
  },
  '0033': {
    id: 'errors.midaz.invalidCodeFormat',
    defaultMessage:
      "The 'code' field must be alphanumeric, in upper case, and must contain at least one letter. Please provide a valid code."
  },
  '0034': {
    id: 'errors.midaz.assetCodeNotFound',
    defaultMessage:
      'The provided asset code does not exist in our records. Please verify the asset code and try again.'
  },
  '0035': {
    id: 'errors.midaz.portfolioIdNotFound',
    defaultMessage:
      'The provided portfolio ID does not exist in our records. Please verify the portfolio ID and try again.'
  },
  '0036': {
    id: 'errors.midaz.segmentIdNotFound',
    defaultMessage:
      'The provided segment ID does not exist in our records. Please verify the segment ID and try again.'
  },
  '0037': {
    id: 'errors.midaz.ledgerIdNotFound',
    defaultMessage:
      'The provided ledger ID does not exist in our records. Please verify the ledger ID and try again.'
  },
  '0038': {
    id: 'errors.midaz.organizationIdNotFound',
    defaultMessage:
      'The provided organization ID does not exist in our records. Please verify the organization ID and try again.'
  },
  '0039': {
    id: 'errors.midaz.parentOrganizationIdNotFound',
    defaultMessage:
      'The provided parent organization ID does not exist in our records. Please verify the parent organization ID and try again.'
  },
  '0040': {
    id: 'errors.midaz.invalidType',
    defaultMessage:
      "The provided 'type' is not valid. Accepted types are: currency, crypto, commodities, or others. Please provide a valid type."
  },
  '0041': {
    id: 'errors.midaz.tokenMissing',
    defaultMessage:
      'A valid token must be provided in the request header. Please include a token and try again.'
  },
  '0042': {
    id: 'errors.midaz.invalidToken',
    defaultMessage:
      'The provided token is invalid or malformed. Please provide a valid token and try again.'
  },
  '0043': {
    id: 'errors.midaz.tokenExpired',
    defaultMessage:
      'The provided token has expired. Please provide a valid token and try again.'
  },
  '0044': {
    id: 'errors.midaz.insufficientPrivileges',
    defaultMessage:
      'You do not have the necessary permissions to perform this action. Please contact your administrator if you believe this is an error.'
  },
  '0045': {
    id: 'errors.midaz.permissionEnforcementError',
    defaultMessage:
      'The enforcer is not configured properly. Please contact your administrator if you believe this is an error.'
  },
  '0046': {
    id: 'errors.midaz.internalServerError',
    defaultMessage:
      'The server encountered an unexpected error. Please try again later or contact support.'
  },
  '0047': {
    id: 'errors.midaz.badRequest',
    defaultMessage:
      'The server could not understand the request due to malformed syntax. Please check the listed fields and try again.'
  },
  '0050': {
    id: 'errors.midaz.metadataKeyLengthExceeded',
    defaultMessage:
      'The metadata key exceeds the maximum allowed length of 100 characters. Please use a shorter key.'
  },
  '0051': {
    id: 'errors.midaz.metadataValueLengthExceeded',
    defaultMessage:
      'The metadata value exceeds the maximum allowed length of 100 characters. Please use a shorter value.'
  },
  '0052': {
    id: 'errors.midaz.accountIdNotFound',
    defaultMessage:
      'The provided account ID does not exist in our records. Please verify the account ID and try again.'
  },
  '0053': {
    id: 'errors.midaz.unexpectedFieldsInTheRequest',
    defaultMessage:
      'The request body contains more fields than expected. Please send only the allowed fields as per the documentation. The unexpected fields are listed in the fields object.'
  },
  '0054': {
    id: 'errors.midaz.noAccountsFound',
    defaultMessage:
      'No accounts were found for the provided account IDs. Please verify the account IDs and try again.'
  },
  '0055': {
    id: 'errors.midaz.assetIdNotFound',
    defaultMessage:
      'The provided asset ID does not exist in our records. Please verify the asset ID and try again.'
  },
  '0056': {
    id: 'errors.midaz.noAssetsFound',
    defaultMessage:
      'No assets were found in the search. Please review the search criteria and try again.'
  },
  '0057': {
    id: 'errors.midaz.noSegmentsFound',
    defaultMessage:
      'No segments were found in the search. Please review the search criteria and try again.'
  },
  '0058': {
    id: 'errors.midaz.noPortfoliosFound',
    defaultMessage:
      'No portfolios were found in the search. Please review the search criteria and try again.'
  },
  '0059': {
    id: 'errors.midaz.noOrganizationsFound',
    defaultMessage:
      'No organizations were found in the search. Please review the search criteria and try again.'
  },
  '0060': {
    id: 'errors.midaz.noLedgersFound',
    defaultMessage:
      'No ledgers were found in the search. Please review the search criteria and try again.'
  },
  '0061': {
    id: 'errors.midaz.balanceUpdateFailed',
    defaultMessage:
      'The balance could not be updated for the specified account ID. Please verify the account ID and try again.'
  },
  '0062': {
    id: 'errors.midaz.noAccountIdsProvided',
    defaultMessage:
      'No account IDs were provided for the balance update. Please provide valid account IDs and try again.'
  },
  '0063': {
    id: 'errors.midaz.failedToRetrieveAccountsByAliases',
    defaultMessage:
      'The accounts could not be retrieved using the specified aliases. Please verify the aliases for accuracy and try again.'
  },
  '0064': {
    id: 'errors.midaz.noAccountsFoundSearch',
    defaultMessage:
      'No accounts were found in the search. Please review the search criteria and try again.'
  },
  '0065': {
    id: 'errors.midaz.invalidPathParameter',
    defaultMessage:
      'The provided path parameter is not in the expected format. Please ensure the parameter adheres to the required format and try again.'
  },
  '0066': {
    id: 'errors.midaz.invalidAccountType',
    defaultMessage:
      "The provided 'type' is not valid. Accepted types are: deposit, savings, loans, marketplace, creditCard or external. Please provide a valid type."
  },
  '0067': {
    id: 'errors.midaz.invalidMetadataNesting',
    defaultMessage:
      'The metadata object cannot contain nested values. Please ensure that the value is not nested and try again.'
  },
  '0068': {
    id: 'errors.midaz.operationIdNotFound',
    defaultMessage:
      'The provided operation ID does not exist in our records. Please verify the operation ID and try again.'
  },
  '0069': {
    id: 'errors.midaz.noOperationsFound',
    defaultMessage:
      'No operations were found in the search. Please review the search criteria and try again.'
  },
  '0070': {
    id: 'errors.midaz.transactionIdNotFound',
    defaultMessage:
      'The provided transaction ID does not exist in our records. Please verify the transaction ID and try again.'
  },
  '0071': {
    id: 'errors.midaz.noTransactionsFound',
    defaultMessage:
      'No transactions were found in the search. Please review the search criteria and try again.'
  },
  '0073': {
    id: 'errors.midaz.transactionValueMismatch',
    defaultMessage:
      'The values for the source, the destination, or both do not match the specified transaction amount. Please verify the values and try again.'
  },
  '0074': {
    id: 'errors.midaz.externalAccountModificationProhibited',
    defaultMessage:
      "Accounts of type 'external' cannot be deleted or modified as they are used for traceability with external systems. Please review your request and ensure operations are only performed on internal accounts."
  },
  '0077': {
    id: 'errors.midaz.invalidDateFormatError',
    defaultMessage:
      "The 'initialDate', 'finalDate', or both are in the incorrect format. Please use the 'yyyy-mm-dd' format and try again."
  },
  '0078': {
    id: 'errors.midaz.invalidFinalDateError',
    defaultMessage:
      "The 'finalDate' cannot be earlier than the 'initialDate'. Please verify the dates and try again."
  },
  '0079': {
    id: 'errors.midaz.dateRangeExceedsLimitError',
    defaultMessage:
      "The range between 'initialDate' and 'finalDate' exceeds the permitted limit of months. Please adjust the dates and try again."
  },
  '0080': {
    id: 'errors.midaz.paginationLimitExceeded',
    defaultMessage:
      'The pagination limit exceeds the maximum allowed of items per page. Please verify the limit and try again.'
  },
  '0081': {
    id: 'errors.midaz.invalidSortOrder',
    defaultMessage:
      "The 'sort_order' field must be 'asc' or 'desc'. Please provide a valid sort order and try again."
  },
  '0082': {
    id: 'errors.midaz.invalidQueryParameter',
    defaultMessage:
      'One or more query parameters are in an incorrect format. Ensure they meet the required format before trying again.'
  },
  '0083': {
    id: 'errors.midaz.invalidDateRangeError',
    defaultMessage:
      "Both 'initialDate' and 'finalDate' fields are required and must be in the 'yyyy-mm-dd' format. Please provide valid dates and try again."
  },
  '0084': {
    id: 'errors.midaz.duplicateIdempotencyKey',
    defaultMessage:
      'The idempotency key is already in use. Please provide a unique key and try again.'
  },
  '0085': {
    id: 'errors.midaz.accountAliasNotFound',
    defaultMessage:
      'The provided account Alias does not exist in our records. Please verify the account Alias and try again.'
  },
  '0086': {
    id: 'errors.midaz.raceConditioningDetected',
    defaultMessage:
      'A race condition was detected while processing your request. Please try again.'
  },
  '0087': {
    id: 'errors.midaz.transactionRevertAlreadyExist',
    defaultMessage: 'Transaction revert already exists. Please try again.'
  },
  '0088': {
    id: 'errors.midaz.transactionIsAlreadyReversal',
    defaultMessage: 'Transaction is already a reversal. Please try again.'
  },
  '0089': {
    id: 'errors.midaz.transactionCantBeReverted',
    defaultMessage: "Transaction can't be reverted. Please try again."
  },
  '0090': {
    id: 'errors.midaz.transactionAmbiguousAccount',
    defaultMessage:
      "Transaction can't use same account in sources and destinations."
  },
  '0091': {
    id: 'errors.midaz.idCannotBeUsedAsParentId',
    defaultMessage:
      'The provided ID cannot be used as the parent ID. Please choose a different one.'
  },
  '0093': {
    id: 'errors.midaz.balanceCannotBeDeleted',
    defaultMessage:
      'Balance cannot be deleted because it still has funds in it.'
  },
  '0095': {
    id: 'errors.midaz.messageBrokerUnavailable',
    defaultMessage:
      'The server encountered an unexpected error while connecting to Message Broker. Please try again later or contact support.'
  },
  '0096': {
    id: 'errors.midaz.invalidAccountAlias',
    defaultMessage:
      'The alias contains invalid characters. Please verify the alias value and try again.'
  },
  '0097': {
    id: 'errors.midaz.overflowError',
    defaultMessage:
      'The request could not be completed due to an overflow. Please check the values, and try again.'
  },
  '0100': {
    id: 'errors.fee.invalidCalculationRequest',
    defaultMessage: 'Invalid fee calculation request'
  },
  '0101': {
    id: 'errors.fee.packageNotFound',
    defaultMessage: 'Fee package not found'
  },
  '0102': {
    id: 'errors.fee.calculationFailed',
    defaultMessage: 'Fee calculation failed'
  },
  '0103': {
    id: 'errors.fee.invalidAssetType',
    defaultMessage: 'Invalid asset type for fee calculation'
  },
  '0104': {
    id: 'errors.fee.insufficientBalance',
    defaultMessage: 'Insufficient balance for fee deduction'
  },
  '0105': {
    id: 'errors.fee.ruleValidationFailed',
    defaultMessage: 'Fee rule validation failed'
  },
  '0106': {
    id: 'errors.fee.conflictingPriorities',
    defaultMessage: 'Conflicting fee priorities'
  },
  '0107': {
    id: 'errors.fee.invalidPercentageSum',
    defaultMessage: 'Invalid percentage sum in fee distribution'
  },
  '0108': {
    id: 'errors.fee.maximumFeeExceeded',
    defaultMessage: 'Maximum fee limit exceeded'
  },
  '0109': {
    id: 'errors.fee.minimumFeeNotMet',
    defaultMessage: 'Minimum fee requirement not met'
  },
  '0110': {
    id: 'errors.fee.invalidReferenceAmount',
    defaultMessage: 'Invalid reference amount for fee calculation'
  },
  '0111': {
    id: 'errors.fee.deductibleConfigError',
    defaultMessage: 'Deductible fee configuration error'
  },
  '0112': {
    id: 'errors.fee.waiverNotApplicable',
    defaultMessage: 'Fee waiver not applicable'
  },
  '0113': {
    id: 'errors.fee.invalidCalculationMethod',
    defaultMessage: 'Invalid fee calculation method'
  },
  '0114': {
    id: 'errors.fee.packageExpired',
    defaultMessage: 'Fee package expired'
  },
  '0115': {
    id: 'errors.fee.packageNotActive',
    defaultMessage: 'Fee package not active'
  },
  '0116': {
    id: 'errors.fee.invalidMaxBetweenTypes',
    defaultMessage: 'Invalid maxBetweenTypes configuration'
  },
  '0117': {
    id: 'errors.fee.calculationTimeout',
    defaultMessage: 'Fee calculation timeout'
  },
  '0118': {
    id: 'errors.fee.circularDependency',
    defaultMessage: 'Circular fee dependency detected'
  },
  '0119': {
    id: 'errors.fee.precisionError',
    defaultMessage: 'Fee calculation precision error'
  },
  '0120': {
    id: 'errors.fee.invalidTransactionStructure',
    defaultMessage: 'Invalid transaction structure'
  },
  '0121': {
    id: 'errors.fee.failedToCalculateFee',
    defaultMessage: 'Failed to calculate fee'
  },
  '0122': {
    id: 'errors.fee.missingTransactionFields',
    defaultMessage: 'Missing required transaction fields'
  },
  '0123': {
    id: 'errors.fee.invalidAccountDistribution',
    defaultMessage: 'Invalid account distribution'
  },
  '0124': {
    id: 'errors.fee.assetMismatch',
    defaultMessage: 'Asset mismatch in transaction'
  },
  '0125': {
    id: 'errors.fee.invalidChartOfAccounts',
    defaultMessage: 'Invalid chart of accounts'
  },
  '0126': {
    id: 'errors.fee.transactionAmountValidationFailed',
    defaultMessage: 'Transaction amount validation failed'
  },
  '0127': {
    id: 'errors.fee.invalidSourceAccount',
    defaultMessage: 'Invalid source account configuration'
  },
  '0128': {
    id: 'errors.fee.invalidDestinationAccount',
    defaultMessage: 'Invalid destination account configuration'
  },
  '0129': {
    id: 'errors.fee.transactionMetadataValidationFailed',
    defaultMessage: 'Transaction metadata validation failed'
  },
  '0130': {
    id: 'errors.fee.packageCreationFailed',
    defaultMessage: 'Package creation failed'
  },
  '0131': {
    id: 'errors.fee.packageUpdateFailed',
    defaultMessage: 'Package update failed'
  },
  '0132': {
    id: 'errors.fee.packageDeletionFailed',
    defaultMessage: 'Package deletion failed'
  },
  '0133': {
    id: 'errors.fee.packageFilteringError',
    defaultMessage: 'Package filtering error'
  },
  '0134': {
    id: 'errors.fee.duplicatePackageName',
    defaultMessage: 'Duplicate package name'
  },
  '0135': {
    id: 'errors.fee.invalidPackageConfiguration',
    defaultMessage: 'Invalid package configuration'
  },
  '0136': {
    id: 'errors.fee.packagePriorityConflict',
    defaultMessage: 'Package priority conflict'
  },
  '0137': {
    id: 'errors.fee.invalidPackageEffectiveDates',
    defaultMessage: 'Invalid package effective dates'
  },
  '0138': {
    id: 'errors.fee.packageDependencyNotFound',
    defaultMessage: 'Package dependency not found'
  },
  '0139': {
    id: 'errors.fee.packageValidationFailed',
    defaultMessage: 'Package validation failed'
  },
  '0140': {
    id: 'errors.fee.externalServiceUnavailable',
    defaultMessage: 'External service unavailable'
  },
  '0141': {
    id: 'errors.fee.databaseConnectionError',
    defaultMessage: 'Database connection error'
  },
  '0142': {
    id: 'errors.fee.configurationError',
    defaultMessage: 'Configuration error'
  },
  '0143': {
    id: 'errors.fee.authenticationFailed',
    defaultMessage: 'Authentication failed'
  },
  '0144': {
    id: 'errors.fee.authorizationFailed',
    defaultMessage: 'Authorization failed'
  },
  '0145': {
    id: 'errors.fee.rateLimitExceeded',
    defaultMessage: 'Rate limit exceeded'
  },
  '0146': {
    id: 'errors.fee.invalidApiVersion',
    defaultMessage: 'Invalid API version'
  },
  '0147': {
    id: 'errors.fee.internalServerError',
    defaultMessage: 'Internal server error'
  },
  '0148': {
    id: 'errors.fee.priority1MustReferenceOriginalAmount',
    defaultMessage: 'Priority 1 fees must reference originalAmount'
  },
  '0149': {
    id: 'errors.fee.priorityGreaterThan1MustReferenceAfterFeesAmount',
    defaultMessage:
      'Priority greater than 1 fees must reference afterFeesAmount'
  },
  '0150': {
    id: 'errors.fee.maxBetweenTypesValidationFailed',
    defaultMessage: 'maxBetweenTypes rule validation failed'
  },
  '0151': {
    id: 'errors.fee.percentageSumValidationFailed',
    defaultMessage: 'Percentage sum validation failed'
  },
  '0152': {
    id: 'errors.fee.assetConsistencyValidationFailed',
    defaultMessage: 'Asset consistency validation failed'
  },
  '0153': {
    id: 'errors.fee.invalidAmountValue',
    defaultMessage: 'Invalid amount value'
  },
  '0154': {
    id: 'errors.fee.missingChartOfAccounts',
    defaultMessage: 'Missing chart of accounts'
  },
  '0155': {
    id: 'errors.fee.invalidDistributionSum',
    defaultMessage: 'Invalid distribution sum'
  },
  '0156': {
    id: 'errors.fee.deductibleFeeValidationFailed',
    defaultMessage: 'Deductible fee validation failed'
  },
  '0157': {
    id: 'errors.fee.routeConsistencyValidationFailed',
    defaultMessage: 'Route consistency validation failed'
  },
  organizationIdRequired: {
    id: 'errors.fee.organizationIdRequired',
    defaultMessage: 'Organization ID is required'
  },
  pluginFeesUrlNotConfigured: {
    id: 'errors.fee.pluginFeesUrlNotConfigured',
    defaultMessage: 'Plugin fees service URL not configured'
  },
  failedToFetchPackageDetails: {
    id: 'errors.fee.failedToFetchPackageDetails',
    defaultMessage: 'Failed to fetch package details'
  },
  feesServiceNotEnabled: {
    id: 'errors.fee.feesServiceNotEnabled',
    defaultMessage: 'Fees service is not enabled'
  },
  feesServiceUrlNotConfigured: {
    id: 'errors.fee.feesServiceUrlNotConfigured',
    defaultMessage: 'Fees service URL not configured'
  },
  feeCalculationFailed: {
    id: 'errors.fee.feeCalculationFailed',
    defaultMessage: 'Fee calculation failed'
  },
  internalServerError: {
    id: 'errors.fee.internalServerError',
    defaultMessage: 'Internal server error'
  },
  noFeesApplied: {
    id: 'fees.noFeesApplied',
    defaultMessage: 'No fees applied'
  },
  feeCalculationFailedNoFees: {
    id: 'fees.calculationFailedNoFees',
    defaultMessage: 'Fee calculation failed - no fees applied'
  },
  deductible: {
    id: 'fees.deductible',
    defaultMessage: 'Deductible'
  },
  nonDeductible: {
    id: 'fees.nonDeductible',
    defaultMessage: 'Non-deductible'
  },
  senderLabel: {
    id: 'fees.senderLabel',
    defaultMessage: 'Sender'
  },
  recipientLabel: {
    id: 'fees.recipientLabel',
    defaultMessage: 'Recipient'
  },
  feePackageDefault: {
    id: 'fees.packageDefault',
    defaultMessage: 'Fee Package'
  },
  feeCollectedByTemplate: {
    id: 'fees.collectedByTemplate',
    defaultMessage: 'Fee collected by {accountAlias}'
  },
  invalidCalculationResponse: {
    id: 'fees.invalidCalculationResponse',
    defaultMessage: 'Invalid calculation response'
  },
  transactionDefault: {
    id: 'fees.transactionDefault',
    defaultMessage: 'Transaction'
  }
})
