import { metadata } from './metadata'
import { z } from 'zod'

const name = z.string().min(3)

const entityId = z.string().min(1).max(255).optional()

export const portfolio = {
  name,
  entityId,
  metadata
}
