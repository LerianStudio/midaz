import { z } from 'zod'
import { metadata } from './metadata'

const title = z.string().min(1).max(50)

const description = z.string().max(250).optional()

const operationType = z.enum(['source', 'destination'])

const ruleType = z.enum(['alias', 'account_type'])

const validIf = z
  .union([
    z.string(),
    z.array(z.string()).min(1, 'At least one account type must be selected')
  ])
  .optional()

const account = z
  .object({
    ruleType,
    validIf
  })
  .nullable()

const id = z.string().uuid()

export const operationRoutes = {
  id,
  title,
  description,
  operationType,
  account,
  metadata
}

export const createOperationRouteSchema = z.object({
  title: operationRoutes.title,
  description: operationRoutes.description,
  operationType: operationRoutes.operationType,
  account: operationRoutes.account,
  metadata: operationRoutes.metadata
})

export const updateOperationRouteSchema = z.object({
  title: operationRoutes.title.optional(),
  description: operationRoutes.description,
  account: operationRoutes.account,
  metadata: operationRoutes.metadata
})

export const operationRouteParamsSchema = z.object({
  organizationId: z.string().uuid(),
  ledgerId: z.string().uuid(),
  operationRouteId: z.string().uuid().optional()
})
