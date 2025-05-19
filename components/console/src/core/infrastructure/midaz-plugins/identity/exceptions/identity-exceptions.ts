import { ApiException, HttpStatus } from '@/lib/http'

export class IdentityApiException extends ApiException {
  constructor(message: string, code: string = '0000', status?: HttpStatus) {
    super(code, 'Identity Exception', message, status)
  }
}
