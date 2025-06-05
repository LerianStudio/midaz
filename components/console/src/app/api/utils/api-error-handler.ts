import { LoggerAggregator } from 'lib-logs'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { MidazApiException } from '@/core/infrastructure/midaz/exceptions/midaz-exceptions'
import { HttpStatus, ApiException } from '@/lib/http'
import { getIntl } from '@/lib/intl'
import { AuthApiException } from '@/core/infrastructure/midaz-plugins/auth/exceptions/auth-exceptions'

export interface ErrorResponse {
  message: string
  status: number
}

export async function apiErrorHandler(error: any): Promise<ErrorResponse> {
  const intl = await getIntl()
  const logger = container.get<LoggerAggregator>(LoggerAggregator)

  const errorMetadata = {
    errorType: error.constructor.name,
    originalMessage: error.message
  }

  if (error instanceof MidazApiException) {
    logger.error(`Midaz error`, errorMetadata)
    return { message: error.message, status: error.getStatus() }
  }

  if (error instanceof AuthApiException) {
    logger.error(`Auth error`, errorMetadata)
    return { message: error.message, status: error.getStatus() }
  }

  if (error instanceof ApiException) {
    logger.error(`Api error`, errorMetadata)
    return { message: error.message, status: error.getStatus() }
  }

  logger.error(`Unknown error`, errorMetadata)
  return {
    message: intl.formatMessage({
      id: 'error.midaz.unknowError',
      defaultMessage: 'Unknown error on Midaz.'
    }),
    status: HttpStatus.INTERNAL_SERVER_ERROR
  }
}
