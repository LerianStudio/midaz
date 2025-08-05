import { z } from 'zod'
import { onlyNumbers, regex } from './regex'

const line1 = z.string().min(1).max(255)

const line2 = z.string().max(255).nullable().optional()

const zipCode = z.coerce
  .string()
  .min(1)
  .max(12)
  .refine(regex(onlyNumbers), {
    params: { id: 'custom_only_numbers' }
  })

const city = z.string().min(1).max(255)

const state = z.string().max(5).nullable().optional()

const country = z.string().min(1).max(5)

export const address = {
  line1,
  line2,
  zipCode,
  city,
  state,
  country
}
