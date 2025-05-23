import { z } from 'zod'

export const metadata = z.record(z.string(), z.any()).nullable()
