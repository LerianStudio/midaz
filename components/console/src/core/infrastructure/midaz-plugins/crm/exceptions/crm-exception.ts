import { ApiException } from '@/lib/http'

export class CrmException extends ApiException {
  constructor(
    message: string,
    code: number,
    error?: string,
    details?: unknown
  ) {
    super(message, code, error, details)
  }
}

export class CrmApiException extends CrmException {
  constructor(message: string, code: number = 500) {
    super(message, code)
  }
}

export class CrmValidationException extends CrmException {
  constructor(message: string, details?: unknown) {
    super(message, 400, 'VALIDATION_ERROR', details)
  }
}

export class CrmNotFoundException extends CrmException {
  constructor(message: string) {
    super(message, 404, 'NOT_FOUND')
  }
}

export class CrmConflictException extends CrmException {
  constructor(message: string) {
    super(message, 409, 'CONFLICT')
  }
}

export class CrmServerException extends CrmException {
  constructor(message: string) {
    super(message, 500, 'INTERNAL_SERVER_ERROR')
  }
}
