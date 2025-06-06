import { OrganizationAvatarRepository } from '@/core/domain/repositories/organization-avatar-repository'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { OrganizationResponseDto } from '../../dto/organization-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'

export interface FetchOrganizationById {
  execute: (organizationId: string) => Promise<OrganizationResponseDto>
}

@injectable()
export class FetchOrganizationByIdUseCase implements FetchOrganizationById {
  constructor(
    @inject(OrganizationRepository)
    private readonly organizationRepository: OrganizationRepository,
    @inject(OrganizationAvatarRepository)
    private readonly organizationAvatarRepository: OrganizationAvatarRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId: string): Promise<OrganizationResponseDto> {
    const organizationEntity =
      await this.organizationRepository.fetchById(organizationId)

    const organizationAvatarEntity =
      await this.organizationAvatarRepository.fetchById(organizationId)

    const organizationResponseDto: OrganizationResponseDto =
      OrganizationMapper.toResponseDto(
        organizationEntity,
        organizationAvatarEntity?.avatar
      )

    return organizationResponseDto
  }
}
