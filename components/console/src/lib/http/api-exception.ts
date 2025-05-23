import { HttpException, HttpStatus } from '@/lib/http'

export class ApiException extends HttpException {
  constructor(
    public readonly code: string,
    public readonly title: string,
    public readonly message: string,
    status: HttpStatus = HttpStatus.INTERNAL_SERVER_ERROR
  ) {
    super(message, status)
  }

  getResponse() {
    return {
      code: this.code,
      title: this.title,
      message: this.message
    }
  }
}

export class BadRequestApiException extends ApiException {
  constructor(message: string) {
    super('0000', 'Bad Request', message, HttpStatus.BAD_REQUEST)
  }
}

export class UnauthorizedApiException extends ApiException {
  constructor(message: string = 'Unauthorized') {
    super('0001', 'Unauthorized', message, HttpStatus.UNAUTHORIZED)
  }
}

export class ForbiddenApiException extends ApiException {
  constructor(message: string) {
    super('0002', 'Forbidden', message, HttpStatus.FORBIDDEN)
  }
}

export class NotFoundApiException extends ApiException {
  constructor(message: string) {
    super('0003', 'Not Found', message, HttpStatus.NOT_FOUND)
  }
}

export class InternalServerErrorApiException extends ApiException {
  constructor(message: string) {
    super(
      '0004',
      'Internal Server Error',
      message,
      HttpStatus.INTERNAL_SERVER_ERROR
    )
  }
}
