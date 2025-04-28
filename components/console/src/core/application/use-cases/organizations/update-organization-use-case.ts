import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { inject, injectable } from 'inversify'
import type {
  CreateOrganizationDto,
  UpdateOrganizationDto,
  OrganizationResponseDto
} from '../../dto/organization-dto'
import { validateAvatar } from '@/core/infrastructure/utils/avatar/validate-avatar'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface UpdateOrganization {
  execute: (
    organizationId: string,
    organization: Partial<UpdateOrganizationDto>
  ) => Promise<OrganizationResponseDto>
}

@injectable()
export class UpdateOrganizationUseCase implements UpdateOrganization {
  constructor(
    @inject(OrganizationRepository)
    private readonly organizationRepository: OrganizationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    organization: Partial<UpdateOrganizationDto>
  ): Promise<OrganizationResponseDto> {
    await validateAvatar(organization.metadata?.avatar)

    const organizationEntity: Partial<OrganizationEntity> =
      OrganizationMapper.toDomain(organization as CreateOrganizationDto)

    const updatedOrganizationEntity = await this.organizationRepository.update(
      organizationId,
      organizationEntity
    )

    return OrganizationMapper.toResponseDto(updatedOrganizationEntity)
  }
}
