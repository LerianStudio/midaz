import { OperationRoutesRepository } from '@/core/domain/repositories/operation-routes-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface DeleteOperationRoutes {
  execute: (
    organizationId: string,
    ledgerId: string,
    operationRouteId: string
  ) => Promise<void>
}

@injectable()
export class DeleteOperationRoutesUseCase implements DeleteOperationRoutes {
  constructor(
    @inject(OperationRoutesRepository)
    private readonly operationRoutesRepository: OperationRoutesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    operationRouteId: string
  ): Promise<void> {
    await this.operationRoutesRepository.delete(
      organizationId,
      ledgerId,
      operationRouteId
    )
  }
}
