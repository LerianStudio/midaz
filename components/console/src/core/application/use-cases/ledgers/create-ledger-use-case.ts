import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import type { CreateLedgerDto } from '../../dto/create-ledger-dto'
import { CreateLedgerRepository } from '@/core/domain/repositories/ledgers/create-ledger-repository'
import { LedgerResponseDto } from '../../dto/ledger-response-dto'
import { inject, injectable } from 'inversify'
import { LedgerMapper } from '../../mappers/ledger-mapper'
import { LogOperation } from '../../decorators/log-operation'

export interface CreateLedger {
  execute: (
    organizationId: string,
    ledger: CreateLedgerDto
  ) => Promise<LedgerResponseDto>
}

@injectable()
export class CreateLedgerUseCase implements CreateLedger {
  constructor(
    @inject(CreateLedgerRepository)
    private readonly createLedgerRepository: CreateLedgerRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledger: CreateLedgerDto
  ): Promise<LedgerResponseDto> {
    const ledgerEntity: LedgerEntity = LedgerMapper.toDomain(ledger)
    const ledgerCreated = await this.createLedgerRepository.create(
      organizationId,
      ledgerEntity
    )

    return LedgerMapper.toResponseDto(ledgerCreated)
  }
}
