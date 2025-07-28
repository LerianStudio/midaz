import { getController } from '@/lib/http/server'
import { LedgerController } from '@/core/application/controllers/ledger-controller'

export const GET = getController(LedgerController, (c) => c.fetchWithAssets)
