import { ApiException } from '@/lib/http'

export class MidazApiException extends ApiException {
  constructor(message: string, code: string = '0000') {
    super(code, 'Midaz Exception', message)
  }
}
