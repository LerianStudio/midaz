import { defineMessages } from '@/lib/intl'

export const identityApiMessages = defineMessages({
  'IDE-1001': {
    id: 'errors.identity.applicationIdNotFound',
    defaultMessage: 'The provided application ID does not exist in our records.'
  },
  'IDE-1002': {
    id: 'errors.identity.noApplicationsFound',
    defaultMessage: 'No applications were found in the search.'
  },
  'IDE-1003': {
    id: 'errors.identity.userIdNotFound',
    defaultMessage: 'The provided user ID does not exist in our records.'
  },
  'IDE-1004': {
    id: 'errors.identity.noUsersFound',
    defaultMessage: 'No users were found in the search.'
  },
  'IDE-1005': {
    id: 'errors.identity.noGroupsFound',
    defaultMessage: 'No groups were found in the search.'
  },
  'IDE-1006': {
    id: 'errors.identity.groupIdNotFound',
    defaultMessage: 'The provided group ID does not exist in our records.'
  },
  'IDE-0001': {
    id: 'errors.identity.missingFieldsInRequest',
    defaultMessage: 'Your request is missing one or more required fields.'
  },
  'IDE-0002': {
    id: 'errors.identity.invalidFieldType',
    defaultMessage: 'The provided field type in the request is invalid.'
  },
  'IDE-0003': {
    id: 'errors.identity.invalidPathParameter',
    defaultMessage: 'One or more path parameters are in an incorrect format.'
  },
  'IDE-0004': {
    id: 'errors.identity.unexpectedFields',
    defaultMessage:
      'The request body contains more fields than expected. Please send only the allowed fields.'
  },
  'IDE-0005': {
    id: 'errors.identity.invalidQueryParameter',
    defaultMessage: 'One or more query parameters are in an incorrect format.'
  },
  'IDE-0006': {
    id: 'errors.identity.internalServerError',
    defaultMessage:
      'The server encountered an unexpected error. Please try again later.'
  },
  'IDE-0007': {
    id: 'errors.identity.badRequest',
    defaultMessage:
      'The server could not understand the request due to malformed syntax.'
  },
  'IDE-0008': {
    id: 'errors.identity.tokenMissing',
    defaultMessage: 'A valid token must be provided in the request header.'
  },
  'IDE-0009': {
    id: 'errors.identity.invalidToken',
    defaultMessage: 'The provided token is expired, invalid or malformed.'
  },
  'IDE-0010': {
    id: 'errors.identity.unmodifiableField',
    defaultMessage: 'Your request includes a field that cannot be modified.'
  },
  'IDE-0011': {
    id: 'errors.identity.immutableField',
    defaultMessage:
      'The field cannot be modified. Please remove it from your request.'
  },
  'IDE-0012': {
    id: 'errors.identity.invalidApplicationName',
    defaultMessage: 'The provided application name is invalid.'
  },
  'IDE-0013': {
    id: 'errors.identity.userIdNotMatch',
    defaultMessage: 'The provided ID does not match the token owner.'
  },
  'IDE-0014': {
    id: 'errors.identity.metadataKeyLengthExceeded',
    defaultMessage: 'The metadata key exceeds the maximum allowed length.'
  },
  'IDE-0015': {
    id: 'errors.identity.metadataValueLengthExceeded',
    defaultMessage: 'The metadata value exceeds the maximum allowed length.'
  },
  'IDE-0016': {
    id: 'errors.identity.invalidMetadataNesting',
    defaultMessage: 'The metadata object cannot contain nested values.'
  },
  'IDE-0020': {
    id: 'errors.identity.passwordTooShort',
    defaultMessage: 'The password must be at least 12 characters long.'
  }
})
