import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import type { CreateLedgerDto } from '../../dto/create-ledger-dto'
import { CreateLedgerRepository } from '@/core/domain/repositories/ledgers/create-ledger-repository'
import { LedgerResponseDto } from '../../dto/ledger-response-dto'
import { inject, injectable } from 'inversify'
import { LedgerMapper } from '../../mappers/ledger-mapper'
import { UpdateOrganizationRepository } from '@/core/domain/repositories/organizations/update-organization-repository'
import { FetchOrganizationByIdRepository } from '@/core/domain/repositories/organizations/fetch-organization-by-id-repository'
import { omit } from 'lodash'
import { LogOperation } from '../../decorators/log-operation'

export interface CompleteOnboarding {
  execute: (
    organizationId: string,
    ledger: CreateLedgerDto
  ) => Promise<LedgerResponseDto>
}

@injectable()
export class CompleteOnboardingUseCase implements CompleteOnboarding {
  constructor(
    @inject(FetchOrganizationByIdRepository)
    private readonly fetchOrganizationByIdRepository: FetchOrganizationByIdRepository,
    @inject(UpdateOrganizationRepository)
    private readonly updateOrganizationRepository: UpdateOrganizationRepository,
    @inject(CreateLedgerRepository)
    private readonly createLedgerRepository: CreateLedgerRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledger: CreateLedgerDto
  ): Promise<LedgerResponseDto> {
    const organization =
      await this.fetchOrganizationByIdRepository.fetchById(organizationId)

    await this.updateOrganizationRepository.updateOrganization(organizationId, {
      metadata: {
        ...organization.metadata,
        onboarding: null
      }
    })

    const ledgerEntity: LedgerEntity = LedgerMapper.toDomain(ledger)
    const ledgerCreated = await this.createLedgerRepository.create(
      organizationId,
      ledgerEntity
    )

    return LedgerMapper.toResponseDto(ledgerCreated)
  }
}
