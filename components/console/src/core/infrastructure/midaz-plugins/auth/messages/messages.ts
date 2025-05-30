import { defineMessages } from '@/lib/intl'

export const authApiMessages = defineMessages({
  'AUT-0010': {
    id: 'errors.auth.metadataKeyLengthExceeded',
    defaultMessage:
      'The metadata key exceeds the maximum allowed length of characters. Please use a shorter key.'
  },
  'AUT-0011': {
    id: 'errors.auth.metadataValueLengthExceeded',
    defaultMessage:
      'The metadata value exceeds the maximum allowed length of characters. Please use a shorter value.'
  },
  'AUT-0012': {
    id: 'errors.auth.invalidMetadataNesting',
    defaultMessage:
      'The metadata object cannot contain nested values. Please ensure that the value is not nested and try again.'
  },
  'AUT-0002': {
    id: 'errors.auth.invalidPathParameter',
    defaultMessage:
      'One or more path parameters are in an incorrect format. Please check the parameters and ensure they meet the required format before trying again.'
  },
  'AUT-0004': {
    id: 'errors.auth.invalidQueryParameter',
    defaultMessage:
      'One or more query parameters are in an incorrect format. Please check the parameters and ensure they meet the required format before trying again.'
  },
  'AUT-0013': {
    id: 'errors.auth.invalidGrantType',
    defaultMessage:
      "The provided 'grantType' is not valid. Accepted grant types are password, client_credentials, refresh_token, or others. Please provide a valid type."
  },
  'AUT-1001': {
    id: 'errors.auth.unsupportedGrantType',
    defaultMessage:
      "The provided 'grantType' is not supported by this application. Please refer to the application's supported grant types."
  },
  'AUT-0006': {
    id: 'errors.auth.tokenMissing',
    defaultMessage:
      'A valid token must be provided in the request header. Please include a token and try again.'
  },
  'AUT-0007': {
    id: 'errors.auth.invalidToken',
    defaultMessage:
      'The provided token is expired, invalid or malformed. Please provide a valid token and try again.'
  },
  'AUT-1003': {
    id: 'errors.auth.invalidToken',
    defaultMessage:
      'The provided token is expired, invalid or malformed. Please provide a valid token and try again.'
  },
  'AUT-1002': {
    id: 'errors.auth.invalidUsernameOrPassword',
    defaultMessage:
      "The provided 'username' or 'password' is incorrect. Please verify the credentials and try again."
  },
  'AUT-1004': {
    id: 'errors.auth.invalidClient',
    defaultMessage:
      "The provided 'clientId' or 'clientSecret' is incorrect. Please verify the credentials and try again."
  },
  'AUT-0014': {
    id: 'errors.auth.grantTypeMissingFields',
    defaultMessage:
      "The provided 'grant_type' is missing required fields. Please refer to the documentation for guidance."
  },
  'AUT-1005': {
    id: 'errors.auth.invalidRefreshToken',
    defaultMessage:
      "The provided 'refreshToken' is invalid, expired or revoked. Please verify the token and try again."
  },
  'AUT-1015': {
    id: 'errors.auth.enforcementSubNotFound',
    defaultMessage:
      "No subject was found for the provided 'sub'. Please refer to the enforcement documentation for guidance on the correct 'sub' value."
  },
  'AUT-0008': {
    id: 'errors.auth.permissionEnforcementError',
    defaultMessage:
      'The enforcer is not configured properly. Please contact your administrator if you believe this is an error.'
  },
  'AUT-0005': {
    id: 'errors.auth.internalServerError',
    defaultMessage:
      'The server encountered an unexpected error. Please try again later or contact support.'
  }
})
