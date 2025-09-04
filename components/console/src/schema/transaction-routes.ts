import { z } from 'zod'
import { metadata } from './metadata'

const title = z.string().min(1).max(50)

const description = z.string().max(250).optional()

const operationRoutes = z.array(z.string().uuid()).min(2)

const id = z.string().uuid()

export const transactionRoutes = {
  id,
  title,
  description,
  operationRoutes,
  metadata
}
