import { getController } from '@/lib/http/get-controller'
import { SegmentController } from '@/core/application/controllers/segment-controller'

export const POST = getController(SegmentController, (c) => c.create)

export const GET = getController(SegmentController, (c) => c.fetchAll)
