import { getController } from '@/lib/http/get-controller'
import { SegmentController } from '@/core/application/controllers/segment-controller'

export const GET = getController(SegmentController, (c) => c.fetchById)

export const PATCH = getController(SegmentController, (c) => c.update)

export const DELETE = getController(SegmentController, (c) => c.delete)
