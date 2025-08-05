import { z } from 'zod'
import { metadata } from './metadata'
import { alphanumericAliasRegex, regex } from './regex'

const name = z.string().min(3).max(255)

const alias = z
  .string()
  .max(255)
  .refine(regex(alphanumericAliasRegex), {
    params: { id: 'custom_alphanumeric_alias' }
  })

const entityId = z.string().max(255)

const assetCode = z.string()

const portfolioId = z.string()

const segmentId = z.string()

const type = z.string()

const allowSending = z.boolean()

const allowReceiving = z.boolean()

export const accounts = {
  name,
  alias,
  entityId,
  assetCode,
  portfolioId,
  segmentId,
  metadata,
  type,
  allowSending,
  allowReceiving
}
