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
      'A race condition was detected while processing your request. Please try again'
  },
  '0087': {
    id: 'errors.midaz.transactionRevertAlreadyExist',
    defaultMessage: 'Transaction revert already exists. Please try again.'
  },
  '0088': {
    id: 'errors.midaz.transactionIsAlreadyReversal',
    defaultMessage: 'Transaction is already a reversal. Please try again'
  },
  '0089': {
    id: 'errors.midaz.transactionCantBeReverted',
    defaultMessage: "Transaction can't be reverted. Please try again"
  },
  '0090': {
    id: 'errors.midaz.transactionAmbiguousAccount',
    defaultMessage:
      "Transaction can't use same account in sources and destinations"
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
  }
})
