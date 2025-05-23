import { ApiException, HttpStatus } from '@/lib/http'

export class AuthApiException extends ApiException {
  constructor(message: string, code: string = '0000', status?: HttpStatus) {
    super(code, 'Auth Exception', message, status)
  }
}
