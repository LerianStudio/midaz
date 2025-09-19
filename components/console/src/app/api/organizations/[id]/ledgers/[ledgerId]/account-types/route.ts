import { getController } from '@/lib/http/server'
import { AccountTypesController } from '@/core/application/controllers/account-types-controller'

export const GET = getController(AccountTypesController, (c) => c.fetchAll)

export const POST = getController(AccountTypesController, (c) => c.create)
