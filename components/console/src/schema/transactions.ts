import { z } from 'zod'
import { metadata } from './metadata'
import { assets } from './assets'

const description = z.string().max(1024)
const chartOfAccounts = z.string().max(255)
const asset = assets.code
const value = z.coerce
  .number()
  .positive()
  .max(1000 * 1000 * 1000 * 1000)

const account = z.string().min(1).max(255)
const percentage = z.number().min(0).max(100)
const percentageOfPercentage = z.number().min(0).max(100)

const share = {
  percentage,
  percentageOfPercentage
}

const source = {
  account,
  asset,
  value,
  description,
  chartOfAccounts,
  metadata
}

export const transaction = {
  description,
  chartOfAccounts,
  value,
  asset,
  source,
  metadata
}
