import { OrganizationAvatarEntity } from '@/core/domain/entities/organization-avatar-entity'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { OrganizationAvatarRepository } from '@/core/domain/repositories/organization-avatar-repository'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { OrganizationAvatarMapper } from '@/core/infrastructure/mongo/mappers/mongo-organization-avatar-mapper'
import { validateImage } from '@/core/infrastructure/utils/avatar/validate-image'
import { inject, injectable } from 'inversify'
import type {
  CreateOrganizationDto,
  OrganizationResponseDto
} from '../../dto/organization-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { getIntl } from '@/lib/intl'
import { IntlShape } from 'react-intl'

export interface CreateOrganization {
  execute: (
    organization: CreateOrganizationDto
  ) => Promise<OrganizationResponseDto>
}

@injectable()
export class CreateOrganizationUseCase implements CreateOrganization {
  constructor(
    @inject(OrganizationRepository)
    private readonly organizationRepository: OrganizationRepository,
    @inject(OrganizationAvatarRepository)
    private readonly organizationAvatarRepository: OrganizationAvatarRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationData: CreateOrganizationDto
  ): Promise<OrganizationResponseDto> {
    const intl = await getIntl()

    const organizationCreated: OrganizationEntity =
      await this.createOrganization(organizationData)

    const organizationAvatarCreated: OrganizationAvatarEntity | undefined =
      await this.createOrganizationAvatar(
        organizationCreated.id!,
        intl,
        organizationData.avatar
      )

    const organizationResponseDto: OrganizationResponseDto =
      OrganizationMapper.toResponseDto(
        organizationCreated,
        organizationAvatarCreated?.avatar
      )

    return organizationResponseDto
  }

  private async createOrganization(
    organizationData: CreateOrganizationDto
  ): Promise<OrganizationEntity> {
    const organizationEntity: OrganizationEntity =
      OrganizationMapper.toDomain(organizationData)

    const organizationCreated =
      await this.organizationRepository.create(organizationEntity)

    return organizationCreated
  }

  private async createOrganizationAvatar(
    organizationId: string,
    intl: IntlShape,
    avatar?: string
  ): Promise<OrganizationAvatarEntity | undefined> {
    if (!avatar) {
      return undefined
    }

    await validateImage(avatar, intl)

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
