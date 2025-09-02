import { OperationRoutesRepository } from '@/core/domain/repositories/operation-routes-repository'
import {
  OperationRoutesDto,
  UpdateOperationRoutesDto
} from '@/core/application/dto/operation-routes-dto'
import { OperationRoutesMapper } from '@/core/application/mappers/operation-routes-mapper'
import { OperationRoutesEntity } from '@/core/domain/entities/operation-routes-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface UpdateOperationRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    operationRouteId: string,
    operationRoute: Partial<UpdateOperationRoutesDto>
  ) => Promise<OperationRoutesDto>
}

@injectable()
export class UpdateOperationRoutesUseCase implements UpdateOperationRoutes {
  constructor(
    @inject(OperationRoutesRepository)
    private readonly operationRoutesRepository: OperationRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    operationRouteId: string,
    operationRoute: Partial<UpdateOperationRoutesDto>
  ): Promise<OperationRoutesDto> {
    const operationRouteEntity: Partial<OperationRoutesEntity> =
      OperationRoutesMapper.toDomain(operationRoute)

    const updatedOperationRoute: OperationRoutesEntity =
      await this.operationRoutesRepository.update(
        organizationId,
        ledgerId,
        operationRouteId,
        operationRouteEntity
      )

    return OperationRoutesMapper.toDto(updatedOperationRoute)
  }
}
