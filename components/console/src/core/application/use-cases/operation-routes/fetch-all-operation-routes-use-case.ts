import { PaginationDto } from '../../dto/pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { OperationRoutesMapper } from '../../mappers/operation-routes-mapper'
import { OperationRoutesEntity } from '@/core/domain/entities/operation-routes-entity'
import {
  OperationRoutesDto,
  type OperationRoutesSearchParamDto
} from '../../dto/operation-routes-dto'
import { OperationRoutesRepository } from '@/core/domain/repositories/operation-routes-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface FetchAllOperationRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    query?: OperationRoutesSearchParamDto
  ) => Promise<PaginationDto<OperationRoutesDto>>
}

@injectable()
export class FetchAllOperationRoutesUseCase implements FetchAllOperationRoutes {
  constructor(
    @inject(OperationRoutesRepository)
    private readonly operationRoutesRepository: OperationRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    query?: OperationRoutesSearchParamDto
  ): Promise<PaginationDto<OperationRoutesDto>> {
    const operationRoutesResult: PaginationEntity<OperationRoutesEntity> =
      await this.operationRoutesRepository.fetchAll(
        organizationId,
        ledgerId,
        query
      )

    return OperationRoutesMapper.toPaginationResponseDto(operationRoutesResult)
  }
}
