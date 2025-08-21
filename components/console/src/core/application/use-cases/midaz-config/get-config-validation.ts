import { inject, injectable } from 'inversify'
import { MidazConfigValidationDto } from '../../dto/midaz-config-dto'
import { LogOperation } from '@/core/infrastructure/logger/decorators'

export interface GetMidazConfigValidation {
  execute: (organization: string, ledger: string) => Promise<MidazConfigValidationDto>
}

@injectable()
export class GetMidazConfigValidationUseCase implements GetMidazConfigValidation {
  constructor() {}

  @LogOperation({ layer: 'application' })
  async execute(organization: string, ledger: string): Promise<MidazConfigValidationDto> {
    const isConfigEnabled = process.env.MIDAZ_ACCOUNT_TYPE_VALIDATION_ENABLED === 'true'

    if (!isConfigEnabled) {
      return {
        isConfigEnabled: false,
        isLedgerAllowed: false
      }
    }

    const accountTypeValidation = process.env.MIDAZ_ACCOUNT_TYPE_VALIDATION

    if (!accountTypeValidation) {
      return {
        isConfigEnabled: true,
        isLedgerAllowed: false
      }
    }

    const validationPairs = accountTypeValidation
      .split(',')
      .map((pair) => pair.trim())

    const currentPair = `${organization}:${ledger}`
    const isLedgerAllowed = validationPairs.includes(currentPair)

    return {
      isConfigEnabled: true,
      isLedgerAllowed
    }
  }
}