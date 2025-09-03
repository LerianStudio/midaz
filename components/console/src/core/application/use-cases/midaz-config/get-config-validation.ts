import { injectable } from 'inversify'
import { MidazConfigDto } from '../../dto/midaz-config-dto'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface GetMidazConfigValidation {
  execute: () => Promise<MidazConfigDto>
}

@injectable()
export class GetMidazConfigValidationUseCase {
  constructor() {}

  @LogOperation({ layer: 'application' })
  async execute(): Promise<MidazConfigDto> {
    const isConfigEnabled =
      process.env.MIDAZ_ACCOUNT_TYPE_VALIDATION_ENABLED === 'true'

    if (!isConfigEnabled) {
      return {
        isConfigEnabled: false,
        config: []
      }
    }

    const accountTypeValidation = process.env.MIDAZ_ACCOUNT_TYPE_VALIDATION

    if (!accountTypeValidation) {
      return {
        isConfigEnabled: true,
        config: []
      }
    }

    const validationPairs = accountTypeValidation
      .split(',')
      .map((pair) => pair.trim())

    const configMap = new Map<string, string[]>()

    validationPairs.forEach((pair) => {
      const [org, ledger] = pair.split(':')
      if (org && ledger) {
        if (!configMap.has(org)) {
          configMap.set(org, [])
        }
        configMap.get(org)?.push(ledger)
      }
    })

    const config = Array.from(configMap.entries()).map(
      ([organization, ledgers]) => ({
        organization,
        ledgers
      })
    )

    return {
      isConfigEnabled: true,
      config
    }
  }
}
