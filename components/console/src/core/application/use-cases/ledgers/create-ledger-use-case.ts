import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import type { CreateLedgerDto } from '../../dto/ledger-dto'
import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import { LedgerDto } from '../../dto/ledger-dto'
import { inject, injectable } from 'inversify'
import { LedgerMapper } from '../../mappers/ledger-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface CreateLedger {
  execute: (
    organizationId: string,
    ledger: CreateLedgerDto
  ) => Promise<LedgerDto>
}

@injectable()
export class CreateLedgerUseCase implements CreateLedger {
  constructor(
    @inject(LedgerRepository)
    private readonly ledgerRepository: LedgerRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledger: CreateLedgerDto
  ): Promise<LedgerDto> {
    const ledgerEntity: LedgerEntity = LedgerMapper.toDomain(ledger)
    const ledgerCreated = await this.ledgerRepository.create(
      organizationId,
      ledgerEntity
    )

    return LedgerMapper.toResponseDto(ledgerCreated)
  }
}
