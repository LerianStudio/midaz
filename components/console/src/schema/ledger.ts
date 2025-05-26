import { z } from 'zod'
import { metadata } from './metadata'

const name = z.string().min(1).max(255)

export const ledger = { name, metadata }
