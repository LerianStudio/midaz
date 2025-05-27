import { ApiException, HttpStatus } from '@/lib/http'

export class CrmException extends ApiException {
  constructor(
    message: string,
    code: string,
    title: string,
    status: HttpStatus = HttpStatus.INTERNAL_SERVER_ERROR
  ) {
    super(code, title, message, status)
  }
}

export class CrmApiException extends CrmException {
  constructor(message: string, status: HttpStatus = HttpStatus.INTERNAL_SERVER_ERROR) {
    super(message, '0100', 'CRM API Error', status)
  }
}

export class CrmValidationException extends CrmException {
  constructor(message: string) {
    super(message, '0101', 'CRM Validation Error', HttpStatus.BAD_REQUEST)
  }
}

export class CrmNotFoundException extends CrmException {
  constructor(message: string) {
    super(message, '0102', 'CRM Not Found', HttpStatus.NOT_FOUND)
  }
}

export class CrmConflictException extends CrmException {
  constructor(message: string) {
    super(message, '0103', 'CRM Conflict', HttpStatus.CONFLICT)
  }
}

export class CrmServerException extends CrmException {
  constructor(message: string) {
    super(message, '0104', 'CRM Server Error', HttpStatus.INTERNAL_SERVER_ERROR)
  }
}
