import { z } from 'zod'

const username = z.string().min(1).max(255)

const password = z.string().min(4).max(255)

export const auth = { username, password }
