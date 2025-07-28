import {
  OrganizationDto,
  type OrganizationSearchParamDto
} from '../../dto/organization-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { PaginationDto } from '../../dto/pagination-dto'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { OrganizationAvatarRepository } from '@/core/domain/repositories/organization-avatar-repository'
import { OrganizationAvatarEntity } from '@/core/domain/entities/organization-avatar-entity'

export interface FetchAllOrganizations {
  execute: (
    query: OrganizationSearchParamDto
  ) => Promise<PaginationDto<OrganizationDto>>
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
    query: OrganizationSearchParamDto
  ): Promise<PaginationDto<OrganizationDto>> {
    const organizations = await this.organizationRepository.fetchAll(query)

    if (!organizations?.items?.length) {
      return OrganizationMapper.toPaginationResponseDto(organizations)
    }

    const organizationIds: string[] = organizations.items.map(
      (organization) => organization.id
    ) as string[]

    const organizationAvatars: OrganizationAvatarEntity[] =
      await this.organizationAvatarRepository.fetchByOrganizationId(
        organizationIds
      )

    return OrganizationMapper.toPaginationResponseDto(
      organizations,
      organizationAvatars
    )
  }
}
