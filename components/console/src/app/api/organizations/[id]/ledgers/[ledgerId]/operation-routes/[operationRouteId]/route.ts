import { getController } from '@/lib/http/server'
import { OperationRoutesController } from '@/core/application/controllers/operation-routes-controller'

export const GET = getController(
  OperationRoutesController,
  (c) => c.fetchById
)

export const PUT = getController(
  OperationRoutesController,
  (c) => c.update
)

export const DELETE = getController(
  OperationRoutesController,
  (c) => c.delete
)