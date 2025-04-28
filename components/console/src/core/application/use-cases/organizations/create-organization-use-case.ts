import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import type {
  CreateOrganizationDto,
  OrganizationResponseDto
} from '../../dto/organization-dto'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { inject, injectable } from 'inversify'
import { validateAvatar } from '@/core/infrastructure/utils/avatar/validate-avatar'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface CreateOrganization {
  execute: (
    organization: CreateOrganizationDto
  ) => Promise<OrganizationResponseDto>
}

@injectable()
export class CreateOrganizationUseCase implements CreateOrganization {
  constructor(
    @inject(OrganizationRepository)
    private readonly organizationRepository: OrganizationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationData: CreateOrganizationDto
  ): Promise<OrganizationResponseDto> {
    await validateAvatar(organizationData.metadata?.avatar)

    const organizationEntity: OrganizationEntity =
      OrganizationMapper.toDomain(organizationData)

    const organizationCreated =
      await this.organizationRepository.create(organizationEntity)

    return OrganizationMapper.toResponseDto(organizationCreated)
  }
}
