import { z } from 'zod'
import { metadata } from './metadata'

const title = z.string().min(1).max(50)

const description = z.string().max(250).optional()

const operationRoutes = z
  .array(z.string().uuid())
  .min(1, 'At least one operation route must be selected')

const id = z.string().uuid()

export const transactionRoutes = {
  id,
  title,
  description,
  operationRoutes,
  metadata
}

export const createTransactionRouteSchema = z.object({
  title: transactionRoutes.title,
  description: transactionRoutes.description,
  operationRoutes: transactionRoutes.operationRoutes,
  metadata: transactionRoutes.metadata
})

export const updateTransactionRouteSchema = z.object({
  title: transactionRoutes.title.optional(),
  description: transactionRoutes.description,
  operationRoutes: transactionRoutes.operationRoutes,
  metadata: transactionRoutes.metadata
})

export const transactionRouteParamsSchema = z.object({
  organizationId: z.string().uuid(),
  ledgerId: z.string().uuid(),
  transactionRouteId: z.string().uuid().optional()
})
