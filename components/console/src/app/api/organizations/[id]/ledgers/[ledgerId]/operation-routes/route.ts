import { getController } from '@/lib/http/server'
import { OperationRoutesController } from '@/core/application/controllers/operation-routes-controller'

export const GET = getController(
  OperationRoutesController,
  (c) => c.fetchAll
)

export const POST = getController(
  OperationRoutesController,
  (c) => c.create
)