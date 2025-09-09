import {
  OperationRoutesEntity,
  OperationRoutesSearchEntity
} from '@/core/domain/entities/operation-routes-entity'
import { OperationRoutesRepository } from '@/core/domain/repositories/operation-routes-repository'
import { injectable, inject } from 'inversify'
import { CursorPaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazOperationRoutesDto } from '../dto/midaz-operation-routes-dto'
import { MidazCursorPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazOperationRoutesMapper } from '../mappers/midaz-operation-routes-mapper'
import { createQueryString } from '@/lib/search'
import { MidazApiException } from '../exceptions/midaz-exceptions'

@injectable()
export class MidazOperationRoutesRepository
  implements OperationRoutesRepository
{
  private baseUrl: string = process.env.MIDAZ_TRANSACTION_BASE_PATH as string

  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    operationRoute: OperationRoutesEntity
  ): Promise<OperationRoutesEntity> {
    const dto = MidazOperationRoutesMapper.toCreateDto(operationRoute)

    const response = await this.httpService.post<MidazOperationRoutesDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazOperationRoutesMapper.toEntity(response)
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    query?: OperationRoutesSearchEntity
  ): Promise<CursorPaginationEntity<OperationRoutesEntity>> {
    const {
      id,
      title,
      cursor,
      limit = 10,
      sortBy = 'createdAt',
      sortOrder = 'asc'
    } = query ?? {}

    const queryParams = createQueryString({
      id,
      title,
      cursor,
      sort_by: sortBy,
      sort_order: sortOrder,
      limit: limit.toString()
    })

    const response = await this.httpService.get<
      MidazCursorPaginationDto<MidazOperationRoutesDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes?${queryParams}`
    )

    return MidazOperationRoutesMapper.toCursorPaginationEntity({
      ...response,
      items: response.items
    })
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    operationRouteId: string
  ): Promise<OperationRoutesEntity> {
    try {
      const response = await this.httpService.get<MidazOperationRoutesDto>(
        `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes/${operationRouteId}`
      )

      return MidazOperationRoutesMapper.toEntity(response)
    } catch (error) {
      if (error instanceof MidazApiException) {
        if (error.code === '0085') {
          return {} as OperationRoutesEntity
        }
      }
      throw error
    }
  }

  async update(
    organizationId: string,
    ledgerId: string,
    operationRouteId: string,
    operationRoute: Partial<OperationRoutesEntity>
  ): Promise<OperationRoutesEntity> {
    const dto = MidazOperationRoutesMapper.toUpdateDto(operationRoute)

    const response = await this.httpService.patch<MidazOperationRoutesDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes/${operationRouteId}`,
      {
        body: JSON.stringify(dto)
      }
    )

    return MidazOperationRoutesMapper.toEntity(response)
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    operationRouteId: string
  ): Promise<void> {
    await this.httpService.delete(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/operation-routes/${operationRouteId}`
    )
  }
}
