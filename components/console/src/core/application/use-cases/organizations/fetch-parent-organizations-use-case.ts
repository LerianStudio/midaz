import { inject, injectable } from 'inversify'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { OrganizationResponseDto } from '../../dto/organization-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchParentOrganizations {
  execute(organizationId?: string): Promise<OrganizationResponseDto[]>
}

@injectable()
export class FetchParentOrganizationsUseCase
  implements FetchParentOrganizations
{
  constructor(
    @inject(OrganizationRepository)
    private readonly organizationRepository: OrganizationRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId?: string): Promise<OrganizationResponseDto[]> {
    const organizations = await this.organizationRepository.fetchAll(100, 1)

    const parentOrganizationsFiltered = organizations.items.filter(
      (organization) => organization.id !== organizationId
    )

    const parentOrganizations: OrganizationResponseDto[] =
      parentOrganizationsFiltered.map(OrganizationMapper.toResponseDto)

    return parentOrganizations
  }
}
