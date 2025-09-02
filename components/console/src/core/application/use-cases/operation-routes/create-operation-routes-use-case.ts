import { OperationRoutesRepository } from '@/core/domain/repositories/operation-routes-repository'
import { OperationRoutesEntity } from '@/core/domain/entities/operation-routes-entity'
import { OperationRoutesMapper } from '../../mappers/operation-routes-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import type {
  CreateOperationRoutesDto,
  OperationRoutesDto
} from '../../dto/operation-routes-dto'

export interface CreateOperationRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    operationRoute: CreateOperationRoutesDto
  ) => Promise<OperationRoutesDto>
}

@injectable()
export class CreateOperationRoutesUseCase implements CreateOperationRoutes {
  constructor(
    @inject(OperationRoutesRepository)
    private readonly operationRoutesRepository: OperationRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    operationRoute: CreateOperationRoutesDto
  ): Promise<OperationRoutesDto> {
    const operationRouteEntity: OperationRoutesEntity =
      OperationRoutesMapper.toDomain(operationRoute)
    const operationRouteCreated = await this.operationRoutesRepository.create(
      organizationId,
      ledgerId,
      operationRouteEntity
    )

    return OperationRoutesMapper.toDto(operationRouteCreated)
  }
}
