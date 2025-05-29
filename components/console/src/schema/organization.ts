import { z } from 'zod'
import { address } from './address'
import { onlyNumbers, regex } from './regex'
import { metadata } from './metadata'

const id = z.string().optional()

const parentOrganizationId = z.string().optional().nullable()

const legalName = z.string().min(1).max(255)

const doingBusinessAs = z.string().min(1).max(100).optional()

const legalDocument = z
  .string()
  .min(1)
  .max(255)
  .refine(regex(onlyNumbers), {
    params: { id: 'custom_only_numbers' }
  })

const accentColor = z.string().optional()

const avatar = z.string().optional()

const status = z
  .object({
    code: z.string(),
    description: z.string()
  })
  .default({
    code: 'ACTIVE',
    description: 'organization is active'
  })

export const organization = {
  id,
  parentOrganizationId,
  legalName,
  doingBusinessAs,
  legalDocument,
  metadata,
  accentColor,
  avatar,
  status,
  address
}
