import { FetchLedgerByIdRepository } from '@/core/domain/repositories/ledgers/fetch-ledger-by-id-repository'
import { LedgerResponseDto } from '../../dto/ledger-response-dto'
import { LedgerMapper } from '../../mappers/ledger-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchLedgerById {
  execute: (
    organizationId: string,
    ledgerId: string
  ) => Promise<LedgerResponseDto>
}

@injectable()
export class FetchLedgerByIdUseCase implements FetchLedgerById {
  constructor(
    @inject(FetchLedgerByIdRepository)
    private readonly fetchLedgerByIdRepository: FetchLedgerByIdRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string
  ): Promise<LedgerResponseDto> {
    const ledgerEntity = await this.fetchLedgerByIdRepository.fetchById(
      organizationId,
      ledgerId
    )

    return LedgerMapper.toResponseDto(ledgerEntity)
  }
}
