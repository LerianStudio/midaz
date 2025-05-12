import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { OrganizationResponseDto } from '../../dto/organization-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationDto } from '../../dto/pagination-dto'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { OrganizationAvatarRepository } from '@/core/domain/repositories/organization-avatar-repository'
import { OrganizationAvatarEntity } from '@/core/domain/entities/organization-avatar-entity'

export interface FetchAllOrganizations {
  execute: (
    limit: number,
    page: number
  ) => Promise<PaginationDto<OrganizationResponseDto>>
}

@injectable()
export class FetchAllOrganizationsUseCase implements FetchAllOrganizations {
  constructor(
    @inject(OrganizationRepository)
    private organizationRepository: OrganizationRepository,
    @inject(OrganizationAvatarRepository)
    private organizationAvatarRepository: OrganizationAvatarRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    limit: number,
    page: number
  ): Promise<PaginationDto<OrganizationResponseDto>> {
    const organizationsResult: PaginationEntity<OrganizationEntity> =
      await this.organizationRepository.fetchAll(limit, page)

    if (!organizationsResult.items.length) {
      return OrganizationMapper.toPaginationResponseDto(organizationsResult)
    }

    const organizationIds: string[] = organizationsResult.items.map(
      (organization) => organization.id
    ) as string[]

    const organizationAvatars: OrganizationAvatarEntity[] =
      await this.organizationAvatarRepository.fetchFilteredByOrganizationIdList(
        organizationIds
      )

    return OrganizationMapper.toPaginationResponseDto(
      organizationsResult,
      organizationAvatars
    )
  }
}
