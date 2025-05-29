import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import { LedgerResponseDto } from '../../dto/ledger-dto'
import { LedgerMapper } from '../../mappers/ledger-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchLedgerById {
  execute: (
    organizationId: string,
    ledgerId: string
  ) => Promise<LedgerResponseDto>
}

@injectable()
export class FetchLedgerByIdUseCase implements FetchLedgerById {
  constructor(
    @inject(LedgerRepository)
    private readonly LedgerRepository: LedgerRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string
  ): Promise<LedgerResponseDto> {
    const ledgerEntity = await this.LedgerRepository.fetchById(
      organizationId,
      ledgerId
    )

    return LedgerMapper.toResponseDto(ledgerEntity)
  }
}
