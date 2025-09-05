import { getController } from '@/lib/http/server'
import { TransactionRoutesController } from '@/core/application/controllers/transaction-routes-controller'

export const GET = getController(TransactionRoutesController, (c) => c.fetchAll)
