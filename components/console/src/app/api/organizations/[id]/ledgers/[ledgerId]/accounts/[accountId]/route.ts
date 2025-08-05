import { getController } from '@/lib/http/server'
import { AccountController } from '@/core/application/controllers/account-controller'

export const GET = getController(AccountController, (c) => c.fetchById)

export const PATCH = getController(AccountController, (c) => c.update)

export const DELETE = getController(AccountController, (c) => c.delete)
