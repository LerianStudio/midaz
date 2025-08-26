import {
  OperationRoutesEntity,
  OperationRoutesSearchEntity
} from '../entities/operation-routes-entity'
import { PaginationEntity } from '../entities/pagination-entity'

export abstract class OperationRoutesRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    operationRoute: OperationRoutesEntity
  ) => Promise<OperationRoutesEntity>
  
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    query?: OperationRoutesSearchEntity
  ) => Promise<PaginationEntity<OperationRoutesEntity>>
  
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    operationRouteId: string
  ) => Promise<OperationRoutesEntity>
  
  abstract update: (
    organizationId: string,
    ledgerId: string,
    operationRouteId: string,
    operationRoute: Partial<OperationRoutesEntity>
  ) => Promise<OperationRoutesEntity>
  
  abstract delete: (
    organizationId: string,
    ledgerId: string,
    operationRouteId: string
  ) => Promise<void>
}
