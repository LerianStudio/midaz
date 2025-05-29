import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import type {
  UpdateLedgerDto,
  LedgerResponseDto,
  CreateLedgerDto
} from '../../dto/ledger-dto'
import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { inject, injectable } from 'inversify'
import { LedgerMapper } from '../../mappers/ledger-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface UpdateLedger {
  execute: (
    organizationId: string,
    ledgerId: string,
    ledger: UpdateLedgerDto
  ) => Promise<LedgerResponseDto>
}

@injectable()
export class UpdateLedgerUseCase implements UpdateLedger {
  constructor(
    @inject(LedgerRepository)
    private readonly ledgerRepository: LedgerRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    ledger: UpdateLedgerDto
  ): Promise<LedgerResponseDto> {
    const ledgerEntity: Partial<LedgerEntity> = LedgerMapper.toDomain(
      ledger as CreateLedgerDto
    )

    const updatedLedgerEntity = await this.ledgerRepository.update(
      organizationId,
      ledgerId,
      ledgerEntity
    )

    return LedgerMapper.toResponseDto(updatedLedgerEntity)
  }
}
