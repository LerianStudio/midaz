import { z } from 'zod'
import { metadata } from './metadata'
import { alphanumericWithDashUnderscoreRegex, regex } from './regex'

const name = z.string().min(3).max(255)

const alias = z
  .string()
  .max(255)
  .refine(regex(alphanumericWithDashUnderscoreRegex), {
    params: { id: 'custom_alphanumeric_with_dash_underscore' }
  })

const entityId = z.string().max(255)

const assetCode = z.string()

const portfolioId = z.string()

const segmentId = z.string()

const type = z.string()

const allowSending = z.boolean()

const allowReceiving = z.boolean()

const balance = z.string()

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
  allowReceiving,
  balance
}
