import { getController } from '@/lib/http/server'
import { HomeController } from '@/core/application/controllers/home-controller'

export const GET = getController(HomeController, (c) => c.getMetrics)
