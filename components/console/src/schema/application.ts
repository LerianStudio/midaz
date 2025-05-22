import { z } from 'zod'

const name = z.string().min(1).max(255)

const description = z.string()

const clientId = z.string().min(1).max(255)

const clientSecret = z.string().min(1)

export const applications = {
  name,
  description,
  clientId,
  clientSecret
}
