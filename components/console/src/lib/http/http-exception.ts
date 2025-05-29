import { HttpStatus } from './http-status'

/**
 * Defines the base HTTP exception, which is handled by the default
 * Exceptions Handler.
 *
 * Inspired by NestJS:
 * https://github.com/nestjs/nest/blob/master/packages/common/exceptions/http.exception.ts
 */
export class HttpException extends Error {
  private readonly status

  constructor(message: string, status?: number) {
    super(message)
    this.status = status || HttpStatus.INTERNAL_SERVER_ERROR
  }

  getStatus() {
    return this.status
  }

  getResponse() {
    return {
      message: this.message
    }
  }
}
