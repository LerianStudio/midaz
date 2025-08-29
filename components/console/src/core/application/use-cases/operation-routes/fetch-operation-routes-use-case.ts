import { OperationRoutesRepository } from '@/core/domain/repositories/operation-routes-repository'
import { OperationRoutesDto } from '../../dto/operation-routes-dto'
import { OperationRoutesMapper } from '../../mappers/operation-routes-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchOperationRoutesById {
  execute: (
    organizationId: string,
    ledgerId: string,
    operationRouteId: string
  ) => Promise<OperationRoutesDto>
}

@injectable()
export class FetchOperationRoutesByIdUseCase
  implements FetchOperationRoutesById
{
  constructor(
    @inject(OperationRoutesRepository)
    private readonly operationRoutesRepository: OperationRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    operationRouteId: string
  ): Promise<OperationRoutesDto> {
    const operationRoute = await this.operationRoutesRepository.fetchById(
      organizationId,
      ledgerId,
      operationRouteId
    )

    return OperationRoutesMapper.toDto(operationRoute)
  }
}
