import { LoggerAggregator } from '@/core/application/logger/logger-aggregator'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { MidazError } from '@/core/infrastructure/errors/midaz-error'
import { HttpStatus, ApiException } from '@/lib/http'
import { getIntl } from '@/lib/intl'

export interface ErrorResponse {
  message: string
  status: number
}

export async function apiErrorHandler(error: any): Promise<ErrorResponse> {
  const intl = await getIntl()
  const midazLogger = container.get(LoggerAggregator)

  const errorMetadata = {
    errorType: error.constructor.name,
    originalMessage: error.message
  }

  if (error instanceof MidazError) {
    midazLogger.error(`Midaz error`, errorMetadata)
    return { message: error.message, status: HttpStatus.BAD_REQUEST }
  }

  if (error instanceof ApiException) {
    midazLogger.error(`Api error`, errorMetadata)
    return { message: error.message, status: error.getStatus() }
  }

  midazLogger.error(`Unknown error`, errorMetadata)
  return {
    message: intl.formatMessage({
      id: 'error.midaz.unknowError',
      defaultMessage: 'Error on Midaz.'
    }),
    status: HttpStatus.INTERNAL_SERVER_ERROR
  }
}
