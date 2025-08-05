import { injectable, injectFromBase } from 'inversify'
import { applyDecorators } from '../utils/apply-decorators'
import { MidazApiException } from '@/core/infrastructure/midaz/exceptions/midaz-exceptions'
import { getIntl } from '@/lib/intl/get-intl'
import { ApiException } from '../..'
import { AuthApiException } from '@/core/infrastructure/midaz-plugins/auth/exceptions/auth-exceptions'
import { DatabaseException } from '@/core/infrastructure/mongo/exceptions/database-exception'
import { HttpStatus } from '../../http-status'
import { NextResponse } from 'next/server'
import { LoggerAggregator } from '@lerianstudio/lib-logs'

export type ErrorResponse = {
  message: string
  status: number
}

async function apiErrorHandler(
  error: any,
  logger: LoggerAggregator
): Promise<ErrorResponse> {
  const intl = await getIntl()

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

  if (error instanceof DatabaseException) {
    logger.error(`Database error`, errorMetadata)
    return { message: error.message, status: HttpStatus.BAD_REQUEST }
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

/**
 * A class decorator that wraps all methods in the class with error handling.
 *
 * @returns
 */
export function Controller(): ClassDecorator {
  return applyDecorators(
    injectable(),
    injectFromBase({
      extendProperties: true
    }),
    function (target: Function) {
      // Get all method names from the prototype
      const prototype = target.prototype
      const methodNames = Object.getOwnPropertyNames(prototype).filter(
        (name) =>
          typeof prototype[name] === 'function' && name !== 'constructor'
      )

      // Replace each method with a wrapped version
      for (const methodName of methodNames) {
        const originalMethod = prototype[methodName]

        // Replace with wrapped method
        prototype[methodName] = async function (...args: any[]) {
          try {
            return await originalMethod.apply(this, args)
          } catch (error: any) {
            const logger = (this as any).logger

            const { message, status } = await apiErrorHandler(error, logger)
            return NextResponse.json({ message }, { status })
          }
        }
      }
    }
  )
}
