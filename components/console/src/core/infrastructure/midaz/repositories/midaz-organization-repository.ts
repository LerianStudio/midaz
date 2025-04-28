import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { injectable, inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'

@injectable()
export class MidazOrganizationRepository implements OrganizationRepository {
  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  private baseUrl: string = process.env.MIDAZ_BASE_PATH + '/organizations'

  async create(
    organizationData: OrganizationEntity
  ): Promise<OrganizationEntity> {
    const response = await this.httpService.post<OrganizationEntity>(
      this.baseUrl,
      {
        body: JSON.stringify(organizationData)
      }
    )

    return response
  }

  async fetchAll(
    limit: number,
    page: number
  ): Promise<PaginationEntity<OrganizationEntity>> {
    const params = new URLSearchParams({
      limit: limit.toString(),
      page: page.toString()
    })
    const url = `${this.baseUrl}?${params.toString()}`

    const response =
      await this.httpService.get<PaginationEntity<OrganizationEntity>>(url)

    return response
  }

  async fetchById(id: string): Promise<OrganizationEntity> {
    const url = `${this.baseUrl}/${id}`

    const response = await this.httpService.get<OrganizationEntity>(url)

    return response
  }

  async update(
    organizationId: string,
    organization: Partial<OrganizationEntity>
  ): Promise<OrganizationEntity> {
    const url = `${this.baseUrl}/${organizationId}`

    const response = await this.httpService.patch<OrganizationEntity>(url, {
      body: JSON.stringify(organization)
    })

    return response
  }

  async delete(id: string): Promise<void> {
    const url = `${this.baseUrl}/${id}`

    await this.httpService.delete(url)
  }
}
