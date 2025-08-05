import { getController } from '@/lib/http/server'
import { TransactionController } from '@/core/application/controllers/transaction-controller'

export const GET = getController(TransactionController, (c) => c.fetchAll)
