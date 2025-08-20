import { z } from 'zod'
import { metadata } from './metadata'

// Key value validation - alphanumeric with underscores and hyphens only
const keyValueRegex = /^[A-Za-z0-9_-]+$/

const name = z.string().min(1).max(100)

const description = z.string().max(500).optional()

const keyValue = z
  .string()
  .min(1)
  .max(50)
  .refine((value) => keyValueRegex.test(value), {
    message:
      "The field 'keyValue' contains invalid characters. Use only letters, numbers, underscores and hyphens."
  })

const id = z.string().uuid()

export const accountTypes = {
  id,
  name,
  description,
  keyValue,
  metadata
}

// Schema for creating account types
export const createAccountTypeSchema = z.object({
  name: accountTypes.name,
  description: accountTypes.description,
  keyValue: accountTypes.keyValue,
  metadata: accountTypes.metadata
})

// Schema for updating account types
export const updateAccountTypeSchema = z.object({
  name: accountTypes.name.optional(),
  description: accountTypes.description,
  metadata: accountTypes.metadata
})

// Schema for account type parameters
export const accountTypeParamsSchema = z.object({
  id: z.string().uuid(),
  ledgerId: z.string().uuid(),
  accountTypeId: z.string().uuid().optional()
})
