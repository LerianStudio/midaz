import { OrganizationAvatarEntity } from '@/core/domain/entities/organization-avatar-entity'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { OrganizationAvatarRepository } from '@/core/domain/repositories/organization-avatar-repository'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { OrganizationAvatarMapper } from '@/core/infrastructure/mongo/mappers/mongo-organization-avatar-mapper'
import { validateAvatar } from '@/core/infrastructure/utils/avatar/validate-avatar'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import type {
  CreateOrganizationDto,
  OrganizationResponseDto,
  UpdateOrganizationDto
} from '../../dto/organization-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'

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
    private readonly organizationRepository: OrganizationRepository,
    @inject(OrganizationAvatarRepository)
    private readonly organizationAvatarRepository: OrganizationAvatarRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    organization: Partial<UpdateOrganizationDto>
  ): Promise<OrganizationResponseDto> {
    const updatedOrganizationEntity = await this.updateOrganization(
      organizationId,
      organization
    )

    const updatedOrganizationAvatarEntity = await this.updateOrganizationAvatar(
      organizationId,
      organization.avatar
    )

    const organizationResponseDto: OrganizationResponseDto =
      OrganizationMapper.toResponseDto(
        updatedOrganizationEntity,
        updatedOrganizationAvatarEntity?.avatar
      )

    return organizationResponseDto
  }

  private async updateOrganization(
    organizationId: string,
    organization: Partial<UpdateOrganizationDto>
  ): Promise<OrganizationEntity> {
    const organizationEntity: Partial<OrganizationEntity> =
      OrganizationMapper.toDomain(organization as CreateOrganizationDto)

    const updatedOrganizationEntity = await this.organizationRepository.update(
      organizationId,
      organizationEntity
    )

    return updatedOrganizationEntity
  }

  private async updateOrganizationAvatar(
    organizationId: string,
    avatar?: string
  ): Promise<OrganizationAvatarEntity | undefined> {
    if (!avatar) {
      return undefined
    }

    await validateAvatar(avatar)

    const organizationAvatarEntity: OrganizationAvatarEntity =
      OrganizationAvatarMapper.toDomain({
        organizationId,
        avatar
      })

    const organizationAvatarCreated =
      await this.organizationAvatarRepository.create(organizationAvatarEntity)

    return OrganizationAvatarMapper.toDomain(organizationAvatarCreated)
  }
}
