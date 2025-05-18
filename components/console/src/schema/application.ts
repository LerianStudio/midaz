import { z } from 'zod'

const name = z.string().min(1).max(255)

const description = z.string().optional()

const clientId = z.string().min(1).max(255)

const clientSecret = z.string().min(1).max(255)

export const applications = {
  name,
  description,
  clientId,
  clientSecret
}
