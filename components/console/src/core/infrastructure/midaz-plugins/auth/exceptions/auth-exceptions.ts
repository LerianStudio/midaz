import { ApiException } from '@/lib/http'

export class AuthApiException extends ApiException {
  constructor(message: string, code: string = '0000') {
    super(code, 'Auth Exception', message)
  }
}
