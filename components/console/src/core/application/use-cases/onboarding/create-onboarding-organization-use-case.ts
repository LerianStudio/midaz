import { CreateOrganizationRepository } from '@/core/domain/repositories/organizations/create-organization-repository'
import type { CreateOrganizationDto } from '../../dto/create-organization-dto'
import { OrganizationResponseDto } from '../../dto/organization-response-dto'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { inject, injectable } from 'inversify'
import { validateAvatar } from '@/core/infrastructure/utils/avatar/validate-avatar'
import { LogOperation } from '../../decorators/log-operation'

export interface CreateOnboardingOrganization {
  execute: (
    organization: CreateOrganizationDto
  ) => Promise<OrganizationResponseDto>
}

@injectable()
export class CreateOnboardingOrganizationUseCase
  implements CreateOnboardingOrganization
{
  constructor(
    @inject(CreateOrganizationRepository)
    private readonly createOrganizationRepository: CreateOrganizationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationData: CreateOrganizationDto
  ): Promise<OrganizationResponseDto> {
    await validateAvatar(organizationData.metadata?.avatar)

    const organizationEntity: OrganizationEntity = OrganizationMapper.toDomain({
      ...organizationData,
      metadata: {
        ...organizationData.metadata,
        onboarding: true
      }
    })

    const organizationCreated =
      await this.createOrganizationRepository.create(organizationEntity)

    return OrganizationMapper.toResponseDto(organizationCreated)
  }
}
