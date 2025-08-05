import { apiErrorMessages } from '@/core/infrastructure/midaz/messages/messages'

export interface RFCError {
  code: string
  message: string
  status: number
  details?: any
}

export class RFCErrorMapper {
  private static readonly ERROR_CODES: Record<string, { status: number }> = {
    '0100': { status: 400 },
    '0101': { status: 404 },
    '0102': { status: 500 },
    '0103': { status: 400 },
    '0104': { status: 402 },
    '0105': { status: 400 },
    '0106': { status: 409 },
    '0107': { status: 400 },
    '0108': { status: 400 },
    '0109': { status: 400 },
    '0110': { status: 400 },
    '0111': { status: 400 },
    '0112': { status: 400 },
    '0113': { status: 400 },
    '0114': { status: 410 },
    '0115': { status: 400 },
    '0116': { status: 400 },
    '0117': { status: 504 },
    '0118': { status: 400 },
    '0119': { status: 400 },

    '0120': { status: 400 },
    '0121': { status: 500 },
    '0122': { status: 400 },
    '0123': { status: 400 },
    '0124': { status: 400 },
    '0125': { status: 400 },
    '0126': { status: 400 },
    '0127': { status: 400 },
    '0128': { status: 400 },
    '0129': { status: 400 },

    '0130': { status: 500 },
    '0131': { status: 500 },
    '0132': { status: 500 },
    '0133': { status: 400 },
    '0134': { status: 409 },
    '0135': { status: 400 },
    '0136': { status: 409 },
    '0137': { status: 400 },
    '0138': { status: 404 },
    '0139': { status: 400 },

    '0140': { status: 503 },
    '0141': { status: 500 },
    '0142': { status: 500 },
    '0143': { status: 401 },
    '0144': { status: 403 },
    '0145': { status: 429 },
    '0146': { status: 400 },
    '0147': { status: 500 },

    '0148': { status: 400 },
    '0149': { status: 400 },
    '0150': { status: 400 },
    '0151': { status: 400 },
    '0152': { status: 400 },
    '0153': { status: 400 },
    '0154': { status: 400 },
    '0155': { status: 400 },
    '0156': { status: 400 },
    '0157': { status: 400 }
  }

  static mapToRFCError(error: any): RFCError {
    if (error.code && this.ERROR_CODES[error.code]) {
      const errorDef = this.ERROR_CODES[error.code]
      const messageDefinition = apiErrorMessages[error.code]
      return {
        code: error.code,
        message:
          error.message || messageDefinition?.defaultMessage || 'Unknown error',
        status: errorDef.status,
        details: error.details
      }
    }

    if (error.message) {
      const message = error.message.toLowerCase()

      if (message.includes('fee') && message.includes('calculation')) {
        return this.createError('0102', error.message)
      }
      if (message.includes('package') && message.includes('not found')) {
        return this.createError('0101')
      }
      if (message.includes('insufficient') && message.includes('balance')) {
        return this.createError('0104')
      }
      if (message.includes('percentage') && message.includes('sum')) {
        return this.createError('0107')
      }
      if (message.includes('priority') && message.includes('conflict')) {
        return this.createError('0106')
      }

      if (message.includes('transaction') && message.includes('invalid')) {
        return this.createError('0120')
      }
      if (message.includes('asset') && message.includes('mismatch')) {
        return this.createError('0124')
      }
      if (message.includes('distribution') && message.includes('invalid')) {
        return this.createError('0123')
      }

      if (message.includes('service') && message.includes('unavailable')) {
        return this.createError('0140')
      }
      if (message.includes('timeout')) {
        return this.createError('0117')
      }
      if (
        message.includes('unauthorized') ||
        message.includes('authentication')
      ) {
        return this.createError('0143')
      }
      if (message.includes('forbidden') || message.includes('authorization')) {
        return this.createError('0144')
      }
    }

    if (error.status || error.statusCode) {
      const status = error.status || error.statusCode
      switch (status) {
        case 400:
          return this.createError('0100', error.message)
        case 401:
          return this.createError('0143', error.message)
        case 403:
          return this.createError('0144', error.message)
        case 404:
          return this.createError('0101', error.message)
        case 409:
          return this.createError('0106', error.message)
        case 429:
          return this.createError('0145', error.message)
        case 500:
          return this.createError('0147', error.message)
        case 503:
          return this.createError('0140', error.message)
        case 504:
          return this.createError('0117', error.message)
        default:
          return this.createError('0147', error.message)
      }
    }

    return this.createError(
      '0147',
      error.message || 'An unexpected error occurred'
    )
  }

  private static createError(code: string, customMessage?: string): RFCError {
    const errorDef = this.ERROR_CODES[code] || this.ERROR_CODES['0147']
    const messageDefinition = apiErrorMessages[code]
    return {
      code,
      message:
        customMessage || messageDefinition?.defaultMessage || 'Unknown error',
      status: errorDef.status
    }
  }

  static isRFCError(error: any): error is RFCError {
    return (
      error &&
      typeof error.code === 'string' &&
      typeof error.message === 'string' &&
      typeof error.status === 'number'
    )
  }

  static formatErrorResponse(error: RFCError): {
    error: { code: string; message: string; details?: any }
  } {
    return {
      error: {
        code: error.code,
        message: error.message,
        ...(error.details && { details: error.details })
      }
    }
  }
}
