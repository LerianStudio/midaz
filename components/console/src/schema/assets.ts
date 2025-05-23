import { z } from 'zod'
import { metadata } from './metadata'
import { uppercaseLettersOnly, regex } from './regex'

const type = z.string().min(1).max(255)
const name = z.string().min(1).max(255)
const code = z
  .string()
  .min(1)
  .max(255)
  .refine(regex(uppercaseLettersOnly), {
    params: { id: 'custom_uppercase_required' }
  })

export const assets = { type, name, code, metadata }
