import { getController } from '@/lib/http/server'
import { AccountTypesController } from '@/core/application/controllers/account-types-controller'

export const GET = getController(AccountTypesController, (c) => c.fetchById)

export const PATCH = getController(AccountTypesController, (c) => c.update)

export const DELETE = getController(AccountTypesController, (c) => c.delete)
