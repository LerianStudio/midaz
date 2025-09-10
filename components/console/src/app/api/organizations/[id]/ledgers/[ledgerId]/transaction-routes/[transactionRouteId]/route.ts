import { getController } from '@/lib/http/server'
import { TransactionRoutesController } from '@/core/application/controllers/transaction-routes-controller'

export const GET = getController(
  TransactionRoutesController,
  (c) => c.fetchById
)

export const PATCH = getController(TransactionRoutesController, (c) => c.update)

export const DELETE = getController(
  TransactionRoutesController,
  (c) => c.delete
)
