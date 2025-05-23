import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { OrganizationResponseDto } from '../../dto/organization-response-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { FetchAllOrganizationsRepository } from '@/core/domain/repositories/organizations/fetch-all-organizations-repository'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { PaginationDto } from '../../dto/pagination-dto'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchAllOrganizations {
  execute: (
    limit: number,
    page: number
  ) => Promise<PaginationDto<OrganizationResponseDto>>
}

@injectable()
export class FetchAllOrganizationsUseCase implements FetchAllOrganizations {
  constructor(
    @inject(FetchAllOrganizationsRepository)
    private fetchAllOrganizationsRepository: FetchAllOrganizationsRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    limit: number,
    page: number
  ): Promise<PaginationDto<OrganizationResponseDto>> {
    const organizationsResult: PaginationEntity<OrganizationEntity> =
      await this.fetchAllOrganizationsRepository.fetchAll(limit, page)

    return OrganizationMapper.toPaginationResponseDto(organizationsResult)
  }
}
