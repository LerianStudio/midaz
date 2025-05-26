import { ApiException, HttpStatus } from '@/lib/http'

export class MidazApiException extends ApiException {
  constructor(message: string, code: string = '0000', status?: HttpStatus) {
    super(code, 'Midaz Exception', message, status)
  }
}
